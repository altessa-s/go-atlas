// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package serverapi_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/test/bufconn"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"
	grpcserver "github.com/altessa-s/go-atlas/transport/grpc/server"
	httpserver "github.com/altessa-s/go-atlas/transport/http/server"
	"github.com/altessa-s/go-atlas/transport/http/server/router/std"
	"github.com/altessa-s/go-atlas/transport/lifecycle"
)

// adapter deliberately declares every method so a new unnameable requirement breaks compilation.
type adapter struct{ srv *grpcserver.Server }

func (a adapter) Start() error                             { return a.srv.Start() }
func (a adapter) Shutdown(ctx context.Context) error       { return a.srv.Shutdown(ctx) }
func (a adapter) Done() <-chan struct{}                    { return a.srv.Done() }
func (a adapter) Err() error                               { return a.srv.Err() }
func (a adapter) RegisterHandlers(h ...grpcserver.Handler) { a.srv.RegisterHandlers(h...) }
func (a adapter) RegisterInterceptors(i ...interceptors.ServerInterceptor) error {
	return a.srv.RegisterInterceptors(i...)
}

func TestPublicServers(t *testing.T) {
	t.Parallel()
	limits := lifecycle.Timeouts{StartupVerification: time.Millisecond, ShutdownGraceful: time.Second, ShutdownWarning: 100 * time.Millisecond}

	g, err := grpcserver.New(grpcserver.WithListener(bufconn.Listen(1024)), grpcserver.WithLifecycleTimeouts(limits))
	require.NoError(t, err)
	require.Equal(t, limits, g.LifecycleTimeouts())
	var service grpcserver.GRPCServer = adapter{g}
	service.RegisterHandlers()
	require.NoError(t, service.RegisterInterceptors())

	h, err := httpserver.New(httpserver.WithRouter(std.New()), httpserver.WithListener(bufconn.Listen(1024)), httpserver.WithLifecycleTimeouts(limits))
	require.NoError(t, err)
	require.Equal(t, limits, h.LifecycleTimeouts())

	for _, tc := range []struct {
		name   string
		server interface {
			Start() error
			Shutdown(context.Context) error
			Done() <-chan struct{}
			Err() error
		}
		running, closed error
	}{
		{"grpc", service, grpcserver.ErrServerAlreadyStarted, grpcserver.ErrServerClosed},
		{"http", h, httpserver.ErrServerAlreadyStarted, httpserver.ErrServerClosed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.NoError(t, tc.server.Start())
			t.Cleanup(func() { require.NoError(t, tc.server.Shutdown(context.WithoutCancel(t.Context()))) })
			require.ErrorIs(t, tc.server.Start(), tc.running)
			require.NoError(t, tc.server.Shutdown(t.Context()))
			select {
			case <-tc.server.Done():
			default:
				t.Fatal("shutdown left Done open")
			}
			require.NoError(t, tc.server.Err())
			require.ErrorIs(t, tc.server.Start(), tc.closed)
		})
	}
}
