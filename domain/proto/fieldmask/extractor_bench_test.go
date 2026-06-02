// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask_test

import (
	"context"
	"testing"

	"github.com/altessa-s/go-atlas/domain/proto/fieldmask"

	"google.golang.org/protobuf/types/known/fieldmaskpb"

	pb "github.com/altessa-s/go-atlas/proto/gen/fieldbehaviortest/v1"
)

func BenchmarkNewUpdateExtractor(b *testing.B) {
	extract := fieldmask.NewUpdateExtractor(
		func(r *pb.NestedUpdateResourceRequest) *fieldmaskpb.FieldMask { return r.GetOptions().GetUpdateMask() },
		func(r *pb.NestedUpdateResourceRequest) *pb.Resource { return r.GetResource() },
		func(r *pb.NestedUpdateResourceRequest, m *fieldmaskpb.FieldMask) {
			r.Options.UpdateMask = m
		},
	)
	req := &pb.NestedUpdateResourceRequest{
		Resource: &pb.Resource{Name: "n"},
		Options: &pb.NestedUpdateResourceRequest_Options{
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"name", "description"}},
		},
	}

	b.ReportAllocs()
	for b.Loop() {
		_, _, _, _ = extract(context.Background(), req)
	}
}

func BenchmarkNewReadExtractor(b *testing.B) {
	extract := fieldmask.NewReadExtractor(
		func(r *pb.NestedGetResourceRequest) *fieldmaskpb.FieldMask { return r.GetOptions().GetReadMask() },
	)
	req := &pb.NestedGetResourceRequest{
		Name: "id",
		Options: &pb.NestedGetResourceRequest_Options{
			ReadMask: &fieldmaskpb.FieldMask{Paths: []string{"name"}},
		},
	}

	b.ReportAllocs()
	for b.Loop() {
		_, _ = extract(context.Background(), req)
	}
}

func BenchmarkDefaultUpdateExtractor(b *testing.B) {
	extract := fieldmask.DefaultUpdateExtractor()
	req := &pb.UpdateResourceRequest{
		Resource:   &pb.Resource{Name: "n", TenantId: "t"},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"name", "tenant_id"}},
	}

	b.ReportAllocs()
	for b.Loop() {
		_, _, _, _ = extract(context.Background(), req)
	}
}

func BenchmarkDefaultReadExtractor(b *testing.B) {
	extract := fieldmask.DefaultReadExtractor()
	req := &pb.GetResourceRequest{
		Name:     "id",
		ReadMask: &fieldmaskpb.FieldMask{Paths: []string{"name", "tenant_id"}},
	}

	b.ReportAllocs()
	for b.Loop() {
		_, _ = extract(context.Background(), req)
	}
}
