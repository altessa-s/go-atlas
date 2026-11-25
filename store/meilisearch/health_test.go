// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package meilisearch_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	coreio "github.com/altessa-s/go-atlas-core/io"
	"github.com/altessa-s/go-atlas/store/meilisearch"
	"github.com/stretchr/testify/require"
)

const available = `{"status":"available"}`

func TestHealthResponses(t *testing.T) {
	t.Parallel()

	const secret = "secret-health-body"
	atLimit := available + strings.Repeat(" ", 64*1024-len(available))
	oversized := atLimit + " "
	for _, tc := range []struct {
		name      string
		prefix    string
		body      string
		redirect  bool
		truncated bool
		status    int
		want      error
		available bool
	}{
		{name: "exact limit", body: atLimit, available: true},
		{name: "oversized", body: oversized, want: coreio.ErrReadLimitExceeded},
		{name: "oversized with prefix", prefix: "/meili", body: oversized, want: coreio.ErrReadLimitExceeded},
		{name: "oversized after redirect", body: oversized, redirect: true, want: coreio.ErrReadLimitExceeded},
		{name: "truncated after valid JSON", body: available, truncated: true, want: io.ErrUnexpectedEOF},
		{name: "missing status", body: "{}"},
		{name: "degraded", body: `{"status":"degraded"}`},
		{name: "invalid JSON", body: secret},
		{name: "no retry on 503", status: http.StatusServiceUnavailable, body: secret},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var serveAvailable atomic.Bool
			var calls atomic.Int64
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.URL.Path == tc.prefix+"/health" && tc.redirect {
					http.Redirect(w, r, "/ready", http.StatusFound)
					return
				}
				if r.URL.Path != tc.prefix+"/health" && !(tc.redirect && r.URL.Path == "/ready") {
					http.NotFound(w, r)
					return
				}
				if serveAvailable.Load() {
					_, _ = io.WriteString(w, available)
					return
				}
				if tc.truncated {
					w.Header().Set("Content-Length", strconv.Itoa(len(tc.body)+1))
				}
				if tc.status != 0 {
					w.WriteHeader(tc.status)
				}
				_, _ = io.WriteString(w, tc.body)
			}))
			defer srv.Close()

			wantCalls := int64(1)
			if tc.redirect {
				wantCalls++
			}
			check := func(err error) {
				t.Helper()
				if tc.available {
					require.NoError(t, err)
					return
				}
				require.ErrorIs(t, err, meilisearch.ErrHealthCheck)
				if tc.want != nil {
					require.ErrorIs(t, err, tc.want)
				}
				require.NotContains(t, err.Error(), secret)
			}

			var logs bytes.Buffer
			host := strings.Replace(srv.URL, "://", "://test-user:secret-url-password@", 1) + tc.prefix
			opts := meilisearch.WithLogger(slog.New(slog.NewTextHandler(&logs, nil)))
			startup, err := meilisearch.New(t.Context(), host, opts)
			check(err)
			if startup != nil {
				require.NoError(t, startup.Close())
			}
			require.Equal(t, wantCalls, calls.Load())

			serveAvailable.Store(true)
			client, err := meilisearch.New(t.Context(), host, opts)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, client.Close()) })
			serveAvailable.Store(false)
			check(client.Health(t.Context()))
			require.Equal(t, 3*wantCalls, calls.Load())
			require.NotContains(t, logs.String(), secret)
			require.NotContains(t, logs.String(), "secret-url-password")
			require.NotContains(t, logs.String(), "test-user")
		})
	}
}

func TestNew_ClosesInjectedIdleTLSConnectionsOnHealthFailure(t *testing.T) {
	t.Parallel()

	idle, closed := make(chan struct{}, 1), make(chan struct{}, 1)
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "invalid JSON")
	}))
	srv.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		switch state {
		case http.StateIdle:
			idle <- struct{}{}
		case http.StateClosed:
			closed <- struct{}{}
		}
	}
	srv.StartTLS()
	defer srv.Close()

	_, err := meilisearch.New(t.Context(), srv.URL, meilisearch.WithHTTPClient(srv.Client()))
	require.ErrorIs(t, err, meilisearch.ErrHealthCheck)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	for _, state := range []<-chan struct{}{idle, closed} {
		select {
		case <-state:
		case <-ctx.Done():
			require.NoError(t, ctx.Err(), "startup failure must close the idle TLS connection")
		}
	}
}
