// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package updatemask_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-aip/fieldmask/update"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/updatemask"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	pb "github.com/altessa-s/go-atlas/proto/gen/fieldbehaviortest/v1"
)

func request(paths ...string) *pb.UpdateResourceRequest {
	return &pb.UpdateResourceRequest{
		Resource:   &pb.Resource{Id: "id-1", Name: "n", Description: "d", CreateTime: "t"},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: paths},
	}
}

// strictDescriptions returns the descriptions of the google.rpc.BadRequest
// detail of err.
func strictDescriptions(t *testing.T, err error) []string {
	t.Helper()

	st, ok := status.FromError(err)
	require.True(t, ok)
	var out []string
	for _, d := range st.Details() {
		if br, ok := d.(*errdetails.BadRequest); ok {
			for _, v := range br.GetFieldViolations() {
				require.Equal(t, "update_mask", v.GetField())
				out = append(out, v.GetDescription())
			}
		}
	}
	return out
}

func TestNormalize_AppliesAndWritesBack(t *testing.T) {
	t.Parallel()

	req := request("description", "create_time")
	res, err := updatemask.New().Normalize(t.Context(), req)
	require.NoError(t, err)
	require.True(t, res.Located)
	require.Equal(t, 1, res.Paths)
	require.Same(t, req.GetResource(), res.Resource)
	require.Equal(t, []string{"description"}, req.GetUpdateMask().GetPaths())
	require.Empty(t, req.GetResource().GetCreateTime(), "OUTPUT_ONLY cleared")
	require.Empty(t, req.GetResource().GetName(), "unmasked field dropped")
	require.Equal(t, "id-1", req.GetResource().GetId(), "IDENTIFIER kept")
}

func TestNormalize_NotLocated(t *testing.T) {
	t.Parallel()

	req := &pb.Resource{Name: "n"}
	res, err := updatemask.New().Normalize(t.Context(), req)
	require.NoError(t, err)
	require.False(t, res.Located)
	require.Equal(t, "n", req.GetName(), "left untouched")

	_, err = updatemask.New(updatemask.WithStrict()).Normalize(t.Context(), req)
	require.Equal(t, codes.InvalidArgument, status.Code(err))
	require.Equal(t, []string{"update_mask is required"}, strictDescriptions(t, err))
}

func TestNormalize_Strict(t *testing.T) {
	t.Parallel()

	n := updatemask.New(updatemask.WithStrict(), updatemask.WithUpdateOptions(update.WithMaxPaths(1)))

	_, err := n.Normalize(t.Context(), request())
	require.Equal(t, []string{"update_mask must contain at least one path"}, strictDescriptions(t, err))

	_, err = n.Normalize(t.Context(), request("name", "description"))
	require.Equal(t, codes.InvalidArgument, status.Code(err))
	descs := strictDescriptions(t, err)
	require.Len(t, descs, 1)
	require.NotRegexp(t, "^: ", descs[0], "a mask-level refusal names no path")

	_, err = n.Normalize(t.Context(), request("description.no_such"))
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestNormalize_Errors(t *testing.T) {
	t.Parallel()

	_, err := updatemask.New().Normalize(t.Context(), request("tenant_id"))
	require.NoError(t, err, "IMMUTABLE path deferred")

	_, err = updatemask.New().Normalize(t.Context(), request("nope"))
	require.Equal(t, codes.InvalidArgument, status.Code(err), "unknown path")
	require.Len(t, strictDescriptions(t, err), 1, "a rejected path names the mask field")

	noWriteback := updatemask.WithExtractor(func(_ context.Context, req proto.Message) (*fieldmaskpb.FieldMask, proto.Message, func(*fieldmaskpb.FieldMask), bool) {
		r := req.(*pb.UpdateResourceRequest)
		return r.GetUpdateMask(), r.GetResource(), nil, true
	})
	req := request("description")
	_, err = updatemask.New(noWriteback).Normalize(t.Context(), req)
	require.Equal(t, codes.Internal, status.Code(err))
	require.Equal(t, "n", req.GetResource().GetName(), "left untouched")

	panicking := updatemask.WithExtractor(func(context.Context, proto.Message) (*fieldmaskpb.FieldMask, proto.Message, func(*fieldmaskpb.FieldMask), bool) {
		panic("boom")
	})
	_, err = updatemask.New(panicking).Normalize(t.Context(), request("description"))
	require.Equal(t, codes.Internal, status.Code(err))
}

// TestNormalize_TrackedRequestUsesSentMask pins that a second normalizer of
// a tracked call applies the mask the client sent, not the one written back:
// a mask naming only dropped paths stays a no-op even for a normalizer that
// implies an empty mask, and the effective mask is the same either way.
func TestNormalize_TrackedRequestUsesSentMask(t *testing.T) {
	t.Parallel()

	ctx := updatemask.Track(updatemask.Track(t.Context()))
	first, second := updatemask.New(), updatemask.New()

	req := request("create_time")
	res, err := first.Normalize(ctx, req)
	require.NoError(t, err)
	require.Zero(t, res.Paths)
	res, err = second.Normalize(ctx, req)
	require.NoError(t, err)
	require.Zero(t, res.Paths, "the sent mask names only an OUTPUT_ONLY path")
	require.Empty(t, req.GetUpdateMask().GetPaths())

	req = request("description", "name")
	_, err = first.Normalize(ctx, req)
	require.NoError(t, err)
	_, err = second.Normalize(ctx, req)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"description", "name"}, req.GetUpdateMask().GetPaths())

	// Untracked, an empty written-back mask still stays empty, even with
	// the deprecated option enabled.
	req = request("create_time")
	_, err = first.Normalize(t.Context(), req)
	require.NoError(t, err)
	res, err = second.Normalize(t.Context(), req)
	require.NoError(t, err)
	require.Zero(t, res.Paths, "an empty mask remains a no-op")
}

// TestNormalize_TrackedRequestReusedAcrossMessages pins that the mask recorded
// for a message of a stream does not outlive it: a handler receiving every
// message into one request gets each applied from its own mask, whether one
// normalizer or two handle the call. An empty mask after a mask naming a
// field stays a no-op instead of clearing that field.
func TestNormalize_TrackedRequestReusedAcrossMessages(t *testing.T) {
	t.Parallel()

	chains := map[string][]*updatemask.Normalizer{
		"one normalizer":  {updatemask.New()},
		"two normalizers": {updatemask.New(), updatemask.New()},
	}
	for name, chain := range chains {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ctx := updatemask.Track(t.Context())
			req := &pb.UpdateResourceRequest{}
			receive := func(msg *pb.UpdateResourceRequest) int {
				proto.Reset(req)
				proto.Merge(req, msg)

				var paths int
				for _, n := range chain {
					res, err := n.Normalize(ctx, req)
					require.NoError(t, err)
					require.True(t, res.Located)
					paths = res.Paths
				}
				return paths
			}

			require.Equal(t, 1, receive(request("description", "create_time")))
			require.Equal(t, []string{"description"}, req.GetUpdateMask().GetPaths())

			require.Zero(t, receive(&pb.UpdateResourceRequest{
				Resource:   &pb.Resource{Id: "id-1", Name: "n2"},
				UpdateMask: &fieldmaskpb.FieldMask{},
			}), "an empty mask changes nothing")
			require.Empty(t, req.GetUpdateMask().GetPaths())
			require.Empty(t, req.GetResource().GetName(), "empty mask filters unselected input")
			require.Equal(t, "id-1", req.GetResource().GetId(), "IDENTIFIER kept")

			require.Equal(t, 1, receive(request("name")))
			require.Equal(t, []string{"name"}, req.GetUpdateMask().GetPaths())
			require.Equal(t, "n", req.GetResource().GetName())
			require.Empty(t, req.GetResource().GetDescription(), "unmasked field dropped")
		})
	}
}

// TestNormalize_TrackedWritebackInPlace pins that the mask recorded as sent
// survives an extractor whose writeback rewrites the sent mask in place.
func TestNormalize_TrackedWritebackInPlace(t *testing.T) {
	t.Parallel()

	inPlace := updatemask.WithExtractor(func(_ context.Context, req proto.Message) (*fieldmaskpb.FieldMask, proto.Message, func(*fieldmaskpb.FieldMask), bool) {
		r := req.(*pb.UpdateResourceRequest)
		return r.GetUpdateMask(), r.GetResource(), func(m *fieldmaskpb.FieldMask) { r.UpdateMask.Paths = m.GetPaths() }, true
	})
	ctx := updatemask.Track(t.Context())
	first, second := updatemask.New(inPlace), updatemask.New(inPlace)

	req := request("create_time")
	_, err := first.Normalize(ctx, req)
	require.NoError(t, err)
	res, err := second.Normalize(ctx, req)
	require.NoError(t, err)
	require.Zero(t, res.Paths, "the sent mask names only an OUTPUT_ONLY path")
}

// The first plan owns explicit clear intent even though its effective mask
// excludes the protected path. Re-preparing the written-back mask loses it.
func TestNormalize_TrackedPlanKeepsClearIntent(t *testing.T) {
	ctx := updatemask.Track(t.Context())
	req := request("tenant_id")
	first, err := updatemask.New().Normalize(ctx, req)
	require.NoError(t, err)
	second, err := updatemask.New().Normalize(ctx, req)
	require.NoError(t, err)
	require.Same(t, first.Plan, second.Plan)
	require.Empty(t, req.UpdateMask.Paths)
	_, err = second.Plan.Complete(&pb.Resource{Id: "id-1", TenantId: "stored"})
	require.ErrorIs(t, err, update.ErrBehaviorViolation)
	require.Contains(t, err.Error(), "tenant_id")
}
