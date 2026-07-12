// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package logger

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"google.golang.org/grpc"
)

var (
	benchResp any
	benchErr  error
)

// benchServerUnary drives the full unary happy path (DrivenInterceptor +
// PostCall) through a [Slog]-backed logger with a handler that returns
// successfully.
func benchServerUnary(b *testing.B, l *slog.Logger) {
	b.Helper()

	unary := ServerInterceptor(Slog(l)).ServerUnaryInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/bench.EchoService/Echo"}
	handler := func(ctx context.Context, req any) (any, error) { return req, nil }
	ctx := b.Context()

	b.ReportAllocs()
	for b.Loop() {
		benchResp, benchErr = unary(ctx, "req", info, handler)
	}
}

// BenchmarkServerUnaryInterceptor_LogLevelDisabled measures the per-request
// cost when the slog handler filters out the Info-level "call finished"
// record: fields are still collected, but nothing is formatted or written.
func BenchmarkServerUnaryInterceptor_LogLevelDisabled(b *testing.B) {
	l := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
	benchServerUnary(b, l)
}

// BenchmarkServerUnaryInterceptor_LogLevelEnabled measures the per-request
// cost when the record passes the level check and is fully formatted and
// written to a discarding destination.
func BenchmarkServerUnaryInterceptor_LogLevelEnabled(b *testing.B) {
	l := slog.New(slog.NewTextHandler(io.Discard, nil))
	benchServerUnary(b, l)
}
