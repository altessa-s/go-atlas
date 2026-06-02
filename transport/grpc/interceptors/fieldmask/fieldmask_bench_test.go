// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask_test

import (
	"context"
	"testing"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/fieldmask"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	pbfieldmask "github.com/altessa-s/go-atlas/domain/proto/fieldmask"
	pb "github.com/altessa-s/go-atlas/proto/gen/fieldbehaviortest/v1"
)

func BenchmarkClassifyMethod(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_ = fieldmask.ClassifyMethod("/x.v1.X/UpdateBucket")
	}
}

func BenchmarkServerInterceptor_UpdateMask(b *testing.B) {
	uni := fieldmask.ServerInterceptor().ServerUnaryInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/x.v1.X/UpdateResource"}
	handler := func(_ context.Context, _ any) (any, error) { return &pb.Resource{}, nil }

	b.ReportAllocs()
	for b.Loop() {
		req := &pb.UpdateResourceRequest{
			Resource:   &pb.Resource{Name: "n"},
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"name", "create_time"}},
		}
		_, _ = uni(context.Background(), req, info, handler)
	}
}

func BenchmarkServerInterceptor_ReadMask(b *testing.B) {
	uni := fieldmask.ServerInterceptor().ServerUnaryInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/x.v1.X/GetResource"}
	resp := &pb.Resource{Id: "id-1", Name: "kept", TenantId: "drop", Description: "drop"}
	handler := func(_ context.Context, _ any) (any, error) { return resp, nil }

	b.ReportAllocs()
	for b.Loop() {
		req := &pb.GetResourceRequest{
			Name:     "id-1",
			ReadMask: &fieldmaskpb.FieldMask{Paths: []string{"name"}},
		}
		_, _ = uni(context.Background(), req, info, handler)
	}
}

func BenchmarkServerInterceptor_CustomUpdateExtractor(b *testing.B) {
	uni := fieldmask.ServerInterceptor(
		fieldmask.WithMethodKind("/x.v1.X/UpdateNestedResource", fieldmask.KindUpdate),
		fieldmask.WithUpdateExtractor("/x.v1.X/UpdateNestedResource", pbfieldmask.NewUpdateExtractor(
			func(r *pb.NestedUpdateResourceRequest) *fieldmaskpb.FieldMask { return r.GetOptions().GetUpdateMask() },
			func(r *pb.NestedUpdateResourceRequest) *pb.Resource { return r.GetResource() },
			func(r *pb.NestedUpdateResourceRequest, m *fieldmaskpb.FieldMask) { r.Options.UpdateMask = m },
		)),
	).ServerUnaryInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/x.v1.X/UpdateNestedResource"}
	handler := func(_ context.Context, _ any) (any, error) { return &pb.Resource{}, nil }

	b.ReportAllocs()
	for b.Loop() {
		req := &pb.NestedUpdateResourceRequest{
			Resource: &pb.Resource{Name: "n"},
			Options: &pb.NestedUpdateResourceRequest_Options{
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"name", "create_time"}},
			},
		}
		_, _ = uni(context.Background(), req, info, handler)
	}
}

func BenchmarkServerInterceptor_Passthrough(b *testing.B) {
	uni := fieldmask.ServerInterceptor().ServerUnaryInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/x.v1.X/Archive"}
	handler := func(_ context.Context, _ any) (any, error) { return &pb.Resource{}, nil }

	b.ReportAllocs()
	for b.Loop() {
		req := &pb.UpdateResourceRequest{
			Resource: &pb.Resource{Name: "n"},
		}
		_, _ = uni(context.Background(), req, info, handler)
	}
}
