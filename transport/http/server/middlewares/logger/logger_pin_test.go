// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package logger

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-net/clientip"
	"github.com/altessa-s/go-atlas/internal/transport/observability"
	"github.com/altessa-s/go-atlas/observability/tracing"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/requestid"

	slogx "github.com/altessa-s/go-slogx"
)

// pinSpanContext is a valid span context with fixed IDs.
type pinSpanContext struct{}

func (pinSpanContext) TraceID() string                { return "trace-1" }
func (pinSpanContext) SpanID() string                 { return "span-1" }
func (pinSpanContext) TraceFlags() tracing.TraceFlags { return 0 }
func (pinSpanContext) IsValid() bool                  { return true }
func (pinSpanContext) IsRemote() bool                 { return false }
func (pinSpanContext) IsSampled() bool                { return false }

// disabledLogHandler drops every record through its Enabled check.
type disabledLogHandler struct{ logged bool }

func (h *disabledLogHandler) Log(context.Context, string, int, slogx.Fields) { h.logged = true }
func (h *disabledLogHandler) Enabled(context.Context, int) bool              { return false }

func fieldMap(fields slogx.Fields) map[string]any {
	m := make(map[string]any, len(fields))
	for _, f := range fields {
		m[f.Key] = f.Value
	}
	return m
}

func TestHandler_Pin(t *testing.T) {
	t.Parallel()

	t.Run("context values and a portless peer are logged", func(t *testing.T) {
		t.Parallel()
		var got map[string]any
		lh := LogHandlerFunc(func(_ context.Context, _ string, _ int, fields slogx.Fields) { got = fieldMap(fields) })
		h := New(lh).Handler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

		req := httptest.NewRequest(http.MethodGet, "/p", nil)
		req.RemoteAddr = "10.0.0.1"
		ctx := clientip.NewContext(req.Context(), netip.MustParseAddr("192.0.2.7"))
		ctx = requestid.NewContext(ctx, "req-1")
		ctx = tracing.ContextWithSpanContext(ctx, pinSpanContext{})
		h.ServeHTTP(httptest.NewRecorder(), req.WithContext(ctx))

		require.Equal(t, "10.0.0.1", got[observability.FieldKeyClientPeerIP])
		require.Equal(t, "192.0.2.7", got[observability.FieldKeyClientRealIP])
		require.Equal(t, "req-1", got[observability.FieldKeyRequestID])
		require.Equal(t, "trace-1", got[observability.FieldKeyTraceID])
		require.Equal(t, "span-1", got[observability.FieldKeySpanID])
	})

	t.Run("the context logger reaches the handler", func(t *testing.T) {
		t.Parallel()
		base := slog.New(slog.DiscardHandler)
		var inner *slog.Logger
		lh := LogHandlerFunc(func(context.Context, string, int, slogx.Fields) {})
		h := New(lh, WithContextLogger(base)).Handler(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			inner = slogx.FromContext(r.Context())
		}))
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/p", nil))
		require.NotNil(t, inner)
	})

	t.Run("a disabled level skips the record", func(t *testing.T) {
		t.Parallel()
		lh := &disabledLogHandler{}
		h := New(lh).Handler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/p", nil))
		require.False(t, lh.logged)
	})
}
