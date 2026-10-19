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

	redacttestv1 "github.com/altessa-s/go-atlas/proto/gen/redacttest/v1"
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

var benchPayload any

// BenchmarkDebugRedact measures the cached descriptor check plus the copy of a message carrying a secret, and the no-copy path of a
// message type without one.
func BenchmarkDebugRedact(b *testing.B) {
	withSecret := &redacttestv1.ListProvidersResponse{Providers: []*redacttestv1.Provider{
		{Name: "a", ApiKey: new("sk-a")}, {Name: "b", ApiKey: new("sk-b")},
	}}
	plain := &redacttestv1.Tree{Value: "v", Left: &redacttestv1.Tree{Value: "w"}}

	b.Run("secret", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			benchPayload = DebugRedact(withSecret)
		}
	})
	b.Run("plain", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			benchPayload = DebugRedact(plain)
		}
	})
}
