// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask_test

import (
	"context"
	"testing"

	"github.com/altessa-s/go-atlas/domain/proto/fieldmask"

	"google.golang.org/grpc/metadata"

	pb "github.com/altessa-s/go-atlas/proto/gen/fieldbehaviortest/v1"
)

func BenchmarkMetadataReadExtractor_HeaderPresent(b *testing.B) {
	extract := fieldmask.MetadataReadExtractor()
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		fieldmask.DefaultMetadataReadMaskHeader, "name,description",
	))
	req := &pb.GetResourceRequest{Name: "id"}

	b.ReportAllocs()
	for b.Loop() {
		_, _ = extract(ctx, req)
	}
}

func BenchmarkMetadataReadExtractor_HeaderAbsent(b *testing.B) {
	extract := fieldmask.MetadataReadExtractor()
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("other", "x"))
	req := &pb.GetResourceRequest{Name: "id"}

	b.ReportAllocs()
	for b.Loop() {
		_, _ = extract(ctx, req)
	}
}

func BenchmarkMetadataReadExtractor_NoMetadata(b *testing.B) {
	extract := fieldmask.MetadataReadExtractor()
	ctx := context.Background()
	req := &pb.GetResourceRequest{Name: "id"}

	b.ReportAllocs()
	for b.Loop() {
		_, _ = extract(ctx, req)
	}
}
