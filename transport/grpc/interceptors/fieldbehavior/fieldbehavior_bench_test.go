// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldbehavior_test

import (
	"context"
	"testing"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/fieldbehavior"

	"google.golang.org/grpc"

	pb "github.com/altessa-s/go-atlas/proto/gen/fieldbehaviortest/v1"
)

func benchUnary(b *testing.B) grpc.UnaryServerInterceptor {
	b.Helper()
	return fieldbehavior.ServerInterceptor().ServerUnaryInterceptor()
}

func benchResource() *pb.Resource {
	return &pb.Resource{
		Id:          "id-1",
		Name:        "my-resource",
		TenantId:    "tenant-1",
		CreateTime:  "2026-01-01T00:00:00Z",
		Password:    "secret",
		Description: "desc",
		Slug:        "my-slug",
	}
}

func BenchmarkServerInterceptor_CreateRequest(b *testing.B) {
	uni := benchUnary(b)
	info := &grpc.UnaryServerInfo{FullMethod: "/x.v1.X/CreateResource"}
	handler := func(_ context.Context, _ any) (any, error) { return &pb.Resource{}, nil }
	ctx := b.Context()

	b.ReportAllocs()

	for b.Loop() {
		req := benchResource()
		if _, err := uni(ctx, req, info, handler); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkServerInterceptor_UpdateRequest(b *testing.B) {
	uni := benchUnary(b)
	info := &grpc.UnaryServerInfo{FullMethod: "/x.v1.X/UpdateResource"}
	handler := func(_ context.Context, _ any) (any, error) { return &pb.Resource{}, nil }
	ctx := b.Context()

	b.ReportAllocs()

	for b.Loop() {
		req := benchResource()
		if _, err := uni(ctx, req, info, handler); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkServerInterceptor_ResponseOnly(b *testing.B) {
	uni := benchUnary(b)
	info := &grpc.UnaryServerInfo{FullMethod: "/x.v1.X/GetResource"}
	handler := func(_ context.Context, _ any) (any, error) { return benchResource(), nil }
	ctx := b.Context()

	b.ReportAllocs()

	for b.Loop() {
		if _, err := uni(ctx, &pb.Resource{}, info, handler); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkClassifyMethod(b *testing.B) {
	method := "/x.v1.X/UpdateBucket"

	b.ReportAllocs()

	for b.Loop() {
		_ = fieldbehavior.ClassifyMethod(method)
	}
}
