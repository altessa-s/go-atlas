// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-aip/fieldmask/update"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/driver"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/fieldmask"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/metrics"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	pb "github.com/altessa-s/go-atlas/proto/gen/fieldbehaviortest/v1"
)

type streamContextKey struct{}
type contextPassthrough struct{}

func (*contextPassthrough) Name() string { return "context-passthrough" }
func (*contextPassthrough) DrivenInterceptor(ctx context.Context) (driver.Driver, context.Context) {
	return driver.NoopDriver(), context.WithValue(ctx, streamContextKey{}, "derived")
}

func TestUpdateStream_PlanThroughMetrics(t *testing.T) {
	t.Parallel()

	normalizer := fieldmask.ServerInterceptor().ServerStreamInterceptor()
	meter := metrics.ServerInterceptor().ServerStreamInterceptor()
	passthrough := interceptors.ServerDrivenInterceptor(&contextPassthrough{}).ServerStreamInterceptor()
	stored := &pb.Resource{Id: "id-1", TenantId: "tenant-1", Description: "before"}
	spec, err := update.NewSpec(stored)
	require.NoError(t, err)

	patch := update.Patch[proto.Message, proto.Message, *int64]{
		Field: "resource", Spec: spec,
		Precondition: update.Int64Precondition[proto.Message]{Version: func(proto.Message) int64 { return 1 }},
		Read:         func(context.Context) (proto.Message, error) { return stored, nil },
		ToProto:      func(m proto.Message) proto.Message { return m },
		Save: func(_ context.Context, m proto.Message, _ update.Plan, _ *int64) (proto.Message, error) {
			stored = m.(*pb.Resource)

			return m, nil
		},
	}

	var previousContext context.Context
	var previousResource proto.Message
	var previousPlan *update.DeferredPlan
	for range 2 {
		base := &seqStream{ctx: t.Context(), recv: []proto.Message{
			&pb.UpdateResourceRequest{Resource: proto.CloneOf(stored), UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"description"}}},
			&pb.UpdateResourceRequest{Resource: &pb.Resource{Id: "id-1"}, UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"tenant_id"}}},
		}}
		info := &grpc.StreamServerInfo{FullMethod: "/x.v1.X/UpdateResource", IsClientStream: true}
		require.NoError(t, normalizer(nil, base, info, func(srv any, stream grpc.ServerStream) error {

			return meter(srv, stream, info, func(srv any, stream grpc.ServerStream) error {
				return passthrough(srv, stream, info, func(_ any, stream grpc.ServerStream) error {
					for i := range 2 {
						req := new(pb.UpdateResourceRequest)
						require.NoError(t, stream.RecvMsg(req))

						ctx := stream.Context()
						require.Equal(t, "derived", ctx.Value(streamContextKey{}))

						plan, ok := update.PlanFromContext(ctx, req.Resource)
						require.True(t, ok)
						require.NotSame(t, previousPlan, plan)

						if previousPlan != nil {
							_, ok := update.PlanFromContext(ctx, previousResource)
							require.False(t, ok)

							old, ok := update.PlanFromContext(previousContext, previousResource)
							require.True(t, ok)
							require.Same(t, previousPlan, old)
						}
						previousContext, previousResource, previousPlan = ctx, req.Resource, plan
						_, err := patch.Run(ctx, req.Resource, "", req.UpdateMask)
						if i == 0 {
							require.NoError(t, err)
						} else {
							require.Empty(t, req.UpdateMask.Paths)
							require.ErrorIs(t, err, update.ErrProtectedField)
							require.ErrorContains(t, err, "tenant_id")
						}
					}

					return nil
				})
			})
		}))
	}
	require.Equal(t, "tenant-1", stored.TenantId)
}
