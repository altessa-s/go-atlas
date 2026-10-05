// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package grpc_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"

	grpcserver "github.com/altessa-s/go-atlas/transport/grpc/server"
	baseserver "github.com/altessa-s/go-atlas/transport/internal/server"
	stdGrpc "google.golang.org/grpc"
)

// blockingStreamHandler registers a bidi stream that blocks until the
// stream context ends, keeping GracefulStop waiting.
type blockingStreamHandler struct {
	started chan struct{}
}

func (h blockingStreamHandler) Register(gs *stdGrpc.Server, _ <-chan struct{}) {
	gs.RegisterService(&stdGrpc.ServiceDesc{
		ServiceName: "test.Blocking",
		HandlerType: (*any)(nil),
		Streams: []stdGrpc.StreamDesc{{
			StreamName:    "Block",
			ServerStreams: true,
			ClientStreams: true,
			Handler: func(_ any, stream stdGrpc.ServerStream) error {
				h.started <- struct{}{}
				<-stream.Context().Done()
				return nil
			},
		}},
	}, struct{}{})
}

// TestServer_Shutdown_TimeoutWithInFlightStream reproduces the force-stop
// path: GracefulStop is still blocked by an open stream when the deadline
// fires, so Shutdown calls Stop and returns while the GracefulStop goroutine
// finishes afterwards. That late completion must not panic.
func TestServer_Shutdown_TimeoutWithInFlightStream(t *testing.T) {
	t.Parallel()

	for range 5 {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)

		h := blockingStreamHandler{started: make(chan struct{}, 1)}
		srv, err := grpcserver.New(grpcserver.WithBaseOptions(baseserver.WithListener(ln)))
		require.NoError(t, err)
		srv.RegisterHandlers(h)
		require.NoError(t, srv.Start())

		conn, err := stdGrpc.NewClient(ln.Addr().String(), stdGrpc.WithTransportCredentials(insecure.NewCredentials()))
		require.NoError(t, err)
		t.Cleanup(func() { _ = conn.Close() })

		stream, err := conn.NewStream(t.Context(), &stdGrpc.StreamDesc{ServerStreams: true, ClientStreams: true}, "/test.Blocking/Block")
		require.NoError(t, err)
		require.NoError(t, stream.SendMsg(&emptypb.Empty{}))
		select {
		case <-h.started:
		case <-time.After(5 * time.Second):
			require.FailNow(t, "stream handler did not start")
		}

		ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
		err = srv.Shutdown(ctx)
		cancel()
		require.ErrorIs(t, err, context.DeadlineExceeded)

		// The GracefulStop goroutine completes its send after Shutdown has
		// returned; there is no hook to await it, so allow it to run. Before
		// the fix that late send panicked and crashed the test binary.
		time.Sleep(10 * time.Millisecond)
	}
}
