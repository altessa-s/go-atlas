// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package errstatus

import (
	"context"
	"fmt"
	"testing"

	"google.golang.org/grpc"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

func BenchmarkFieldErrorType(b *testing.B) {
	unary := ServerInterceptor(
		WithFieldErrorType(func(e *testFieldError) (string, string) { return e.field, e.reason }),
	).ServerUnaryInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/bench.EchoService/Echo"}
	handlerErr := fmt.Errorf("create: %w", &testFieldError{field: "provider.baseUrl", reason: "required"})
	handler := func(context.Context, any) (any, error) { return nil, handlerErr }
	ctx := b.Context()

	b.ReportAllocs()
	for b.Loop() {
		benchResp, benchErr = unary(ctx, nil, info, handler)
	}
}

// BenchmarkCanonicalRules converts an error matched by the last canonical
// rule, the worst case of the ordered scan.
func BenchmarkCanonicalRules(b *testing.B) {
	unary := ServerInterceptor(WithCanonicalRules()).ServerUnaryInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/bench.EchoService/Echo"}
	handlerErr := fmt.Errorf("lease: %w", coreerrs.ErrBusy)
	handler := func(context.Context, any) (any, error) { return nil, handlerErr }
	ctx := b.Context()

	b.ReportAllocs()
	for b.Loop() {
		benchResp, benchErr = unary(ctx, nil, info, handler)
	}
}

func BenchmarkCamelPath(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_ = CamelPath("reference_item.attrs.min_submission_days")
	}
}

func BenchmarkBadRequest(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_ = BadRequest("provider.baseUrl", "required")
	}
}
