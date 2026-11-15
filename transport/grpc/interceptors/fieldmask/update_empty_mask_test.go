// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-aip/fieldmask/update"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	pb "github.com/altessa-s/go-atlas/proto/gen/fieldbehaviortest/v1"
)

// Explicit empty masks remain no-ops.
func TestServerInterceptor_EmptyUpdateMask(t *testing.T) {
	for _, resource := range []*pb.Resource{
		{}, {Name: "n", TenantId: "changed"}, {Id: "id-1", Profile: &pb.Profile{Id: "p-1", DisplayName: "a"}},
		{Name: "n", CreateTime: "client"}, collectionResource(),
	} {
		req := &pb.UpdateResourceRequest{Resource: proto.CloneOf(resource), UpdateMask: &fieldmaskpb.FieldMask{}}
		_, err := newUnary(t)(t.Context(), req, &grpc.UnaryServerInfo{FullMethod: "/x.v1.X/UpdateResource"}, func(ctx context.Context, _ any) (any, error) {
			pending, ok := update.PlanFromContext(ctx, req.Resource)
			require.True(t, ok)
			mask, err := pending.Complete(&pb.Resource{Name: "stored"})
			require.NoError(t, err)
			require.Empty(t, mask)
			require.Empty(t, req.UpdateMask.Paths)
			return &pb.Resource{}, nil
		})
		require.NoError(t, err)
	}
}
