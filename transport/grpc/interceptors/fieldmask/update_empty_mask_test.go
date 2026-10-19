// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/proto-gen-go/io/altessa/badrequest/v1"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/fieldmask"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	pb "github.com/altessa-s/go-atlas/proto/gen/fieldbehaviortest/v1"
)

// TestServerInterceptor_EmptyUpdateMask_NoOptOff confirms the default: without
// WithApplyEmptyUpdateMask, a wire update_mask with zero paths clears the
// resource (an explicit empty mask updates nothing). No synthesis, and no
// field-behavior violation since validation only inspects masked fields.
func TestServerInterceptor_EmptyUpdateMask_NoOptOff(t *testing.T) {
	t.Parallel()

	uni := newUnary(t) // option NOT set
	req := &pb.UpdateResourceRequest{
		Resource:   &pb.Resource{Name: "n", TenantId: "altered"},
		UpdateMask: &fieldmaskpb.FieldMask{}, // present, zero paths
	}

	seenReq, _, err := runUnary(t, uni, "/x.v1.X/UpdateResource", req, &pb.Resource{}, nil)
	require.NoError(t, err, "empty mask without opt-in clears the resource without violation")

	got, ok := seenReq.(*pb.UpdateResourceRequest)
	require.True(t, ok)
	require.Empty(t, got.GetResource().GetName(), "empty mask clears populated fields")
	require.Empty(t, got.GetResource().GetTenantId(), "empty mask clears populated fields")
	require.Empty(t, got.GetUpdateMask().GetPaths(), "empty mask remains empty on writeback")
}

// TestServerInterceptor_EmptyUpdateMask_SynthesisesFromSetFields covers
// the happy path: when WithApplyEmptyUpdateMask is on, the interceptor
// builds a mask from FromSetFields(resource) and writes the cleaned
// version back. Editable populated fields end up in the writeback mask.
func TestServerInterceptor_EmptyUpdateMask_SynthesisesFromSetFields(t *testing.T) {
	t.Parallel()

	uni := newUnary(t, fieldmask.WithApplyEmptyUpdateMask())
	req := &pb.UpdateResourceRequest{
		Resource:   &pb.Resource{Name: "renamed", Description: "edited"},
		UpdateMask: &fieldmaskpb.FieldMask{},
	}

	seenReq, _, err := runUnary(t, uni, "/x.v1.X/UpdateResource", req, &pb.Resource{}, nil)
	require.NoError(t, err)

	got, ok := seenReq.(*pb.UpdateResourceRequest)
	require.True(t, ok)

	paths := got.GetUpdateMask().GetPaths()
	slices.Sort(paths)
	require.Equal(t, []string{"description", "name"}, paths,
		"synthesised mask must cover the populated editable fields")
}

// TestServerInterceptor_EmptyUpdateMask_KeepsIdentifier pins that the
// synthesised mask leaves the IDENTIFIER out: it addresses the resource
// (AIP-203) and is kept in the body, rather than being read as an attempt to
// change it.
func TestServerInterceptor_EmptyUpdateMask_KeepsIdentifier(t *testing.T) {
	t.Parallel()

	uni := newUnary(t, fieldmask.WithApplyEmptyUpdateMask())
	req := &pb.UpdateResourceRequest{
		Resource:   &pb.Resource{Id: "id-1", Name: "renamed"},
		UpdateMask: &fieldmaskpb.FieldMask{},
	}

	seenReq, _, err := runUnary(t, uni, "/x.v1.X/UpdateResource", req, &pb.Resource{}, nil)
	require.NoError(t, err)

	got, ok := seenReq.(*pb.UpdateResourceRequest)
	require.True(t, ok)
	require.Equal(t, "id-1", got.GetResource().GetId(), "IDENTIFIER kept in the body")
	require.Equal(t, []string{"name"}, got.GetUpdateMask().GetPaths(), "IDENTIFIER left out of the synthesised mask")
}

// TestServerInterceptor_EmptyUpdateMask_KeepsNestedIdentifier pins that an
// IDENTIFIER inside a populated singular sub-message stays out of the
// synthesised mask too: the update succeeds, the nested id is kept in the
// body, and only the editable nested field is masked.
func TestServerInterceptor_EmptyUpdateMask_KeepsNestedIdentifier(t *testing.T) {
	t.Parallel()

	uni := newUnary(t, fieldmask.WithApplyEmptyUpdateMask())
	req := &pb.UpdateResourceRequest{
		Resource: &pb.Resource{
			Id:      "id-1",
			Name:    "renamed",
			Profile: &pb.Profile{Id: "p-1", DisplayName: "Ann"},
		},
		UpdateMask: &fieldmaskpb.FieldMask{},
	}

	seenReq, _, err := runUnary(t, uni, "/x.v1.X/UpdateResource", req, &pb.Resource{}, nil)
	require.NoError(t, err, "a nested IDENTIFIER must not be read as an attempt to change it")

	got, ok := seenReq.(*pb.UpdateResourceRequest)
	require.True(t, ok)
	require.Equal(t, "p-1", got.GetResource().GetProfile().GetId(), "nested IDENTIFIER kept in the body")
	require.Equal(t, "Ann", got.GetResource().GetProfile().GetDisplayName())

	paths := got.GetUpdateMask().GetPaths()
	slices.Sort(paths)
	require.Equal(t, []string{"name", "profile.display_name"}, paths)
}

// TestServerInterceptor_EmptyUpdateMask_NestedIdentifierOnly pins that a
// sub-message whose only populated field is its IDENTIFIER is left out of
// the synthesised mask instead of becoming a whole-field replacement.
func TestServerInterceptor_EmptyUpdateMask_NestedIdentifierOnly(t *testing.T) {
	t.Parallel()

	uni := newUnary(t, fieldmask.WithApplyEmptyUpdateMask())
	req := &pb.UpdateResourceRequest{
		Resource: &pb.Resource{
			Name:    "renamed",
			Profile: &pb.Profile{Id: "p-1"},
		},
		UpdateMask: &fieldmaskpb.FieldMask{},
	}

	seenReq, _, err := runUnary(t, uni, "/x.v1.X/UpdateResource", req, &pb.Resource{}, nil)
	require.NoError(t, err)

	got, ok := seenReq.(*pb.UpdateResourceRequest)
	require.True(t, ok)
	require.Equal(t, []string{"name"}, got.GetUpdateMask().GetPaths(),
		"profile must not be masked (and so replaced) when it carries only its identifier")
}

// TestServerInterceptor_EmptyUpdateMask_StripsOutputOnly verifies that
// OUTPUT_ONLY fields populated on the resource (e.g. create_time echoed
// by a misbehaving client) are dropped from the synthesised mask before
// the handler sees it — ApplyUpdateMask handles the strip on the
// synthesised path the same way it does on an explicit mask.
func TestServerInterceptor_EmptyUpdateMask_StripsOutputOnly(t *testing.T) {
	t.Parallel()

	uni := newUnary(t, fieldmask.WithApplyEmptyUpdateMask())
	req := &pb.UpdateResourceRequest{
		Resource:   &pb.Resource{Name: "n", CreateTime: "2026-01-01T00:00:00Z"},
		UpdateMask: &fieldmaskpb.FieldMask{},
	}

	seenReq, _, err := runUnary(t, uni, "/x.v1.X/UpdateResource", req, &pb.Resource{}, nil)
	require.NoError(t, err)

	got, ok := seenReq.(*pb.UpdateResourceRequest)
	require.True(t, ok)
	require.Equal(t, []string{"name"}, got.GetUpdateMask().GetPaths(),
		"OUTPUT_ONLY entries must be removed from the synthesised writeback mask")
}

// TestServerInterceptor_EmptyUpdateMask_ImmutableRejected ensures the
// synthesised mask goes through the same field-behavior validation as an
// explicit one: an IMMUTABLE field populated on the resource raises
// InvalidArgument with a BadRequest detail, not a silent pass.
func TestServerInterceptor_EmptyUpdateMask_ImmutableRejected(t *testing.T) {
	t.Parallel()

	uni := newUnary(t, fieldmask.WithApplyEmptyUpdateMask())
	req := &pb.UpdateResourceRequest{
		Resource:   &pb.Resource{Name: "n", TenantId: "altered"},
		UpdateMask: &fieldmaskpb.FieldMask{},
	}

	_, _, err := runUnary(t, uni, "/x.v1.X/UpdateResource", req, &pb.Resource{}, nil)
	require.Error(t, err)
	require.Equal(t, codes.InvalidArgument, status.Code(err))

	st, ok := status.FromError(err)
	require.True(t, ok)

	var found *badrequestv1.BadRequest
	for _, d := range st.Details() {
		_, isGoogle := d.(*errdetails.BadRequest)
		require.False(t, isGoogle, "google.rpc.BadRequest must not be emitted")
		if br, isBR := d.(*badrequestv1.BadRequest); isBR {
			found = br
		}
	}
	require.NotNil(t, found, "BadRequest detail must be attached")
	require.Len(t, found.GetFieldViolations(), 1)
	require.Equal(t, "tenant_id", found.GetFieldViolations()[0].GetFieldPath())
	require.Contains(t, found.GetFieldViolations()[0].GetMessage(), "immutable")
}

// TestServerInterceptor_EmptyUpdateMask_NoPopulatedFields covers the
// degenerate case: opt-in is on but the resource itself is empty. There is
// nothing to synthesize, so the interceptor returns early and leaves the
// already-empty request untouched.
func TestServerInterceptor_EmptyUpdateMask_NoPopulatedFields(t *testing.T) {
	t.Parallel()

	uni := newUnary(t, fieldmask.WithApplyEmptyUpdateMask())
	req := &pb.UpdateResourceRequest{
		Resource:   &pb.Resource{},
		UpdateMask: &fieldmaskpb.FieldMask{},
	}

	seenReq, _, err := runUnary(t, uni, "/x.v1.X/UpdateResource", req, &pb.Resource{}, nil)
	require.NoError(t, err)

	got, ok := seenReq.(*pb.UpdateResourceRequest)
	require.True(t, ok)
	require.Empty(t, got.GetUpdateMask().GetPaths())
}

// TestServerInterceptor_NonEmptyUpdateMask_OptInIgnored confirms an
// explicit non-empty mask is never replaced by the synthesised one — the
// opt-in only fires for the empty-mask case.
func TestServerInterceptor_NonEmptyUpdateMask_OptInIgnored(t *testing.T) {
	t.Parallel()

	uni := newUnary(t, fieldmask.WithApplyEmptyUpdateMask())
	req := &pb.UpdateResourceRequest{
		Resource:   &pb.Resource{Name: "n", Description: "edited"},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"name"}},
	}

	seenReq, _, err := runUnary(t, uni, "/x.v1.X/UpdateResource", req, &pb.Resource{}, nil)
	require.NoError(t, err)

	got, ok := seenReq.(*pb.UpdateResourceRequest)
	require.True(t, ok)
	require.Equal(t, []string{"name"}, got.GetUpdateMask().GetPaths(),
		"explicit mask must survive the synthesis path")
}
