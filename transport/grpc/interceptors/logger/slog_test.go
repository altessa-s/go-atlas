// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package logger

import (
	"context"
	"log/slog"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// countingHandler counts records that reach Handle while honoring a level.
type countingHandler struct {
	level   slog.Level
	handled *atomic.Int64
}

func (h countingHandler) Enabled(_ context.Context, l slog.Level) bool { return l >= h.level }
func (h countingHandler) Handle(context.Context, slog.Record) error {
	h.handled.Add(1)
	return nil
}
func (h countingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h countingHandler) WithGroup(string) slog.Handler      { return h }

// TestSlog_ImplementsLevelChecker pins the optional capability and its
// code→level mapping against a level-filtering handler.
func TestSlog_ImplementsLevelChecker(t *testing.T) {
	t.Parallel()

	l := slog.New(countingHandler{level: slog.LevelWarn, handled: &atomic.Int64{}})
	checker, ok := Slog(l).(LevelChecker)
	require.True(t, ok, "Slog logger must implement LevelChecker")

	ctx := t.Context()
	require.False(t, checker.Enabled(ctx, codes.OK), "OK maps to Info, filtered by a Warn handler")
	require.True(t, checker.Enabled(ctx, codes.NotFound), "NotFound maps to Warn")
	require.True(t, checker.Enabled(ctx, codes.Internal), "Internal maps to Error")
}

// TestServerInterceptor_LevelCheckSkipsFilteredRecords drives the full unary
// path and pins that filtered-out completions emit nothing while loggable
// ones still do.
func TestServerInterceptor_LevelCheckSkipsFilteredRecords(t *testing.T) {
	t.Parallel()

	handled := &atomic.Int64{}
	l := slog.New(countingHandler{level: slog.LevelError, handled: handled})

	unary := ServerInterceptor(Slog(l)).ServerUnaryInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/test.EchoService/Echo"}
	ctx := t.Context()

	_, err := unary(ctx, "req", info, func(_ context.Context, req any) (any, error) {
		return req, nil
	})
	require.NoError(t, err)
	require.Zero(t, handled.Load(), "OK completion maps to Info and must be skipped by an Error-level sink")

	wantErr := status.Error(codes.Internal, "boom")
	_, err = unary(ctx, "req", info, func(context.Context, any) (any, error) {
		return nil, wantErr
	})
	require.Error(t, err)
	require.Equal(t, int64(1), handled.Load(), "Internal completion maps to Error and must be logged")
}
