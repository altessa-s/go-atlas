// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

type allowAllLimiter struct{}

func (allowAllLimiter) Allow(context.Context, string) error { return nil }

// http.Client.CloseIdleConnections must reach the pooled transport through
// the retry, circuit-breaker and limiter wrappers; otherwise idle keep-alive
// connections (and their goroutines) outlive a discarded client.
func TestNew_CloseIdleConnectionsReachesTransport(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		opts []Option
	}{
		{name: "default chain", opts: []Option{WithRetryMax(0), WithoutProxy()}},
		{name: "with limiter", opts: []Option{WithRetryMax(0), WithoutProxy(), WithLimiter(allowAllLimiter{})}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var closed atomic.Int32
			srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte("ok"))
			}))
			srv.Config.ConnState = func(_ net.Conn, state http.ConnState) {
				if state == http.StateClosed {
					closed.Add(1)
				}
			}
			srv.Start()
			t.Cleanup(srv.Close)

			c := New(tc.opts...)
			resp, err := c.Get(srv.URL)
			require.NoError(t, err)
			_, _ = io.Copy(io.Discard, resp.Body)
			require.NoError(t, resp.Body.Close())
			require.Zero(t, closed.Load(), "precondition: the connection is kept alive")

			c.CloseIdleConnections()
			testhelpers.WaitFor(t, 2*time.Second, func() bool { return closed.Load() == 1 },
				"CloseIdleConnections did not close the pooled connection")
		})
	}
}
