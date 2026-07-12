// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package geoacl

import (
	"net/netip"
	"testing"

	"github.com/altessa-s/go-atlas/transport/internal/clientip"

	"google.golang.org/grpc"
)

var benchErr error

// BenchmarkUnaryInterceptor_Allowed measures the per-request passthrough cost
// of the unary interceptor on the happy path, using an in-process fake
// resolver so no GeoIP database is involved.
func BenchmarkUnaryInterceptor_Allowed(b *testing.B) {
	inter := ServerUnaryInterceptor(newTestResolver(), newTestRegistry())
	ctx := clientip.NewContext(b.Context(), netip.MustParseAddr("10.1.1.1"))
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Allowed"}

	b.ReportAllocs()
	for b.Loop() {
		_, benchErr = inter(ctx, nil, info, noopHandler)
	}
}

// BenchmarkUnaryInterceptor_IgnoredMethod measures the passthrough cost when
// the method is on the ignore list and geo checks are skipped entirely.
func BenchmarkUnaryInterceptor_IgnoredMethod(b *testing.B) {
	inter := ServerUnaryInterceptor(newTestResolver(), newTestRegistry(), WithIgnoreMethods("/test.Service/Denied"))
	ctx := clientip.NewContext(b.Context(), netip.MustParseAddr("1.2.3.4"))
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Denied"}

	b.ReportAllocs()
	for b.Loop() {
		_, benchErr = inter(ctx, nil, info, noopHandler)
	}
}
