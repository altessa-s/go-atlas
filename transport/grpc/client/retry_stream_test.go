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

// TestRetryDoesNotImposeDeadline streams for longer than the product of the
// retry attempts and the maximum backoff: enabling retries must not cut calls.
func TestRetryDoesNotImposeDeadline(t *testing.T) {
	t.Parallel()

	const (
		messages = 12
		interval = 100 * time.Millisecond
	)
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
				ticker := time.NewTicker(interval)
				defer ticker.Stop()
				for range messages {
					select {
					case <-stream.Context().Done():
						return stream.Context().Err()
					case <-ticker.C:
					}
					if err := stream.SendMsg(wrapperspb.String("tick")); err != nil {
						return err
					}
				}
				return nil
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
		client.WithRetry(client.WithRetryMaxAttempts(2), client.WithRetryMaxBackoff(interval)),
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

	received := 0
	start := time.Now()
	for {
		out := &wrapperspb.StringValue{}
		err := stream.RecvMsg(out)
		if err == io.EOF {
			break
		}
		require.NoError(t, err, "stream cut after %d messages", received)
		received++
	}
	require.Equal(t, messages, received)
	require.Greater(t, time.Since(start), time.Second)
}
