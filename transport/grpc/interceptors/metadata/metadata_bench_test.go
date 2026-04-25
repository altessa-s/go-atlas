// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metadata

import (
	"testing"

	"google.golang.org/grpc"
)

func BenchmarkNewCallMetadata_Unary(b *testing.B) {
	ctx := b.Context()
	info := &grpc.UnaryServerInfo{FullMethod: "/bench.Service/Method"}
	for b.Loop() {
		NewCallMetadata(ctx, info.FullMethod, info)
	}
}

func BenchmarkNewCallMetadataFromMethod(b *testing.B) {
	ctx := b.Context()
	for b.Loop() {
		NewCallMetadataFromMethod(ctx, "/bench.Service/Method")
	}
}

func BenchmarkEnsureInContext(b *testing.B) {
	info := &grpc.UnaryServerInfo{FullMethod: "/bench.Service/Method"}
	ctx := b.Context()
	ctx, _ = EnsureInContext(ctx, info.FullMethod, info)
	for b.Loop() {
		EnsureInContext(ctx, info.FullMethod, info)
	}
}
