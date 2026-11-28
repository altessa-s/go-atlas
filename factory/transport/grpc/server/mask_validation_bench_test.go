// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-aip/fieldmask/update"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/fieldmask"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	grpcconfig "github.com/altessa-s/go-atlas/config/grpc"
	pb "github.com/altessa-s/go-atlas/proto/gen/maskvalidationtest/v1"
)

func BenchmarkMaskValidationFactoryChain(b *testing.B) {
	const service = "/maskvalidationtest.v1.SettingsService/"
	spec, err := update.NewSpec(new(pb.Settings))
	require.NoError(b, err)
	builder := New(&grpcconfig.Config{Interceptors: &grpcconfig.Interceptors{
		FieldMask: &grpcconfig.FieldMaskInterceptor{BaseInterceptor: enabled},
	}}).UseFieldMaskOptions(fieldmask.WithUpdateSpec(service+"UpdateRegistered", spec)).WithInterceptors()
	chain, err := builder.resolveInterceptors()
	require.NoError(b, err)
	ordered, err := interceptors.OrderServerInterceptors(chain...)
	require.NoError(b, err)
	for _, method := range []string{"GetSettings", "ListSettings", "UpdateSettings", "UpdateRegistered"} {
		b.Run(method, func(b *testing.B) {
			info := &grpc.UnaryServerInfo{FullMethod: service + method}
			run := grpc.UnaryHandler(func(_ context.Context, req any) (any, error) { return req, nil })
			for _, o := range slices.Backward(ordered) {
				unary, next := o.ServerUnaryInterceptor(), run
				run = func(ctx context.Context, req any) (any, error) {
					return unary(ctx, req, info, next)
				}
			}
			req := &pb.UpdateSettingsRequest{
				Settings:        &pb.Settings{General: &pb.General{TimeZone: "UTC"}},
				OutsideRequired: new("valid"),
			}
			reset := func() {
				if method == "UpdateSettings" || method == "UpdateRegistered" {
					req.UpdateMask = &fieldmaskpb.FieldMask{Paths: []string{"general.time_zone"}}
				}
			}
			reset()
			_, err := run(b.Context(), req)
			require.NoError(b, err)
			b.ReportAllocs()
			for b.Loop() {
				reset()
				if _, err := run(b.Context(), req); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
