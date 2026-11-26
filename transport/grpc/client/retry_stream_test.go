// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client_test

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/grpc/client"

	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// TestRetryDoesNotImposeDeadline holds a stream open across the product of
// the retry attempts and the maximum backoff: enabling retries must not cut
// calls. The server sends only when the test releases it, so the outcome does
// not depend on scheduling.
func TestRetryDoesNotImposeDeadline(t *testing.T) {
	t.Parallel()

	const (
		backoff = 100 * time.Millisecond
		// held keeps the stream open well past attempts*backoff, the former implicit deadline.
		held = 6 * backoff
	)
	release := make(chan struct{})
	lis := bufconn.Listen(1 << 20)
	desc := grpc.ServiceDesc{
		ServiceName: "test.Ticker",
		HandlerType: (*any)(nil),
		Streams: []grpc.StreamDesc{{
			StreamName:    "Watch",
			ServerStreams: true,
			Handler: func(_ any, stream grpc.ServerStream) error {
				in := &wrapperspb.StringValue{}
				if err := stream.RecvMsg(in); err != nil {
					return err
				}
				if err := stream.SendMsg(wrapperspb.String("first")); err != nil {
					return err
				}
				select {
				case <-stream.Context().Done():
					return stream.Context().Err()
				case <-release:
				}
				return stream.SendMsg(wrapperspb.String("second"))
			},
		}},
	}
	gs := grpc.NewServer()
	gs.RegisterService(&desc, struct{}{})
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)

	c, err := client.New(t.Context(), "passthrough:///ticker",
		client.WithInsecure(), client.WithoutHealthCheck(),
		client.WithDialOptions(grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) })),
		client.WithRetry(client.WithRetryMaxAttempts(2), client.WithRetryMaxBackoff(backoff)),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close(context.WithoutCancel(t.Context())) })

	conn, err := c.GetConnection(t.Context())
	require.NoError(t, err)
	defer c.ReturnConnection(conn)

	stream, err := conn.NewStream(t.Context(), &grpc.StreamDesc{ServerStreams: true}, "/test.Ticker/Watch")
	require.NoError(t, err)
	require.NoError(t, stream.SendMsg(wrapperspb.String("go")))
	require.NoError(t, stream.CloseSend())

	start := time.Now()
	out := &wrapperspb.StringValue{}
	require.NoError(t, stream.RecvMsg(out))
	require.Equal(t, "first", out.GetValue())

	time.Sleep(held - time.Since(start))
	close(release)

	require.NoError(t, stream.RecvMsg(out), "stream cut by an implicit deadline")
	require.Equal(t, "second", out.GetValue())
	require.Equal(t, io.EOF, stream.RecvMsg(out))
}
