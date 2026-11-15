// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-aip/fieldbehavior"
	"github.com/altessa-s/go-aip/fieldmask/update"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/fieldmask"

	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	pb "github.com/altessa-s/go-atlas/proto/gen/fieldbehaviortest/v1"
)

func TestInterceptorUpdatePolicyAndMaskName(t *testing.T) {
	t.Parallel()
	resolver := update.WithResolver(fieldbehavior.ResolverFunc(func(fd protoreflect.FieldDescriptor) []annotations.FieldBehavior {
		if fd.Name() == "name" {
			return []annotations.FieldBehavior{annotations.FieldBehavior_IDENTIFIER}
		}
		return nil
	}))
	uni := newUnary(t, fieldmask.WithUpdateOptions(resolver))
	req := &pb.UpdateResourceRequest{Resource: &pb.Resource{Name: "identity", Description: "edit"}, UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"description"}}}
	_, err := uni(t.Context(), req, &grpc.UnaryServerInfo{FullMethod: "/x.v1.X/UpdateResource"}, func(context.Context, any) (any, error) { return &pb.Resource{}, nil })
	require.NoError(t, err)
	require.Equal(t, "identity", req.Resource.Name)
	require.Equal(t, []string{"description"}, req.UpdateMask.Paths)
	uni = newUnary(t, fieldmask.WithStrictMode(), fieldmask.WithMaskFieldName("changes"))
	_, err = uni(t.Context(), req, &grpc.UnaryServerInfo{FullMethod: "/x.v1.X/UpdateResource"}, func(context.Context, any) (any, error) { require.FailNow(t, "unexpected handler"); return nil, nil })
	require.Error(t, err)
	found := false
	for _, detail := range status.Convert(err).Details() {
		if bad, ok := detail.(*errdetails.BadRequest); ok {
			require.Equal(t, "changes", bad.FieldViolations[0].Field)
			found = true
		}
	}
	require.True(t, found)
}
