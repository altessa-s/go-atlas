// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldbehavior_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/fieldbehavior"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pbfieldbehavior "github.com/altessa-s/go-atlas/domain/proto/fieldbehavior"
	pb "github.com/altessa-s/go-atlas/proto/gen/fieldbehaviortest/v1"
)

func TestClassifyMethod(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		method string
		want   fieldbehavior.Kind
	}{
		{"create", "/x.v1.X/CreateBucket", fieldbehavior.KindCreate},
		{"batch_create", "/x.v1.X/BatchCreateBuckets", fieldbehavior.KindCreate},
		{"update", "/x.v1.X/UpdateBucket", fieldbehavior.KindUpdate},
		{"patch", "/x.v1.X/PatchBucket", fieldbehavior.KindUpdate},
		{"batch_update", "/x.v1.X/BatchUpdateBuckets", fieldbehavior.KindUpdate},
		{"get", "/x.v1.X/GetBucket", fieldbehavior.KindNone},
		{"list", "/x.v1.X/ListBuckets", fieldbehavior.KindNone},
		{"delete", "/x.v1.X/DeleteBucket", fieldbehavior.KindNone},
		{"custom_action", "/x.v1.X/Archive", fieldbehavior.KindNone},
		{"empty", "", fieldbehavior.KindNone},
		{"no_slash", "CreateBucket", fieldbehavior.KindCreate},
		{"case_sensitive", "/x.v1.X/createBucket", fieldbehavior.KindNone},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, fieldbehavior.ClassifyMethod(tc.method))
		})
	}
}

func TestKindString(t *testing.T) {
	t.Parallel()

	require.Equal(t, "none", fieldbehavior.KindNone.String())
	require.Equal(t, "create", fieldbehavior.KindCreate.String())
	require.Equal(t, "update", fieldbehavior.KindUpdate.String())
	require.Equal(t, "skip", fieldbehavior.KindSkip.String())
	require.Equal(t, "Kind(99)", fieldbehavior.Kind(99).String())
}

// fullResource builds a Resource with every annotated field populated. Field
// behaviors come from proto/fieldbehaviortest/v1/test.proto:
//
//	id           IDENTIFIER
//	name         REQUIRED
//	tenant_id    IMMUTABLE
//	create_time  OUTPUT_ONLY
//	password     INPUT_ONLY
//	description  (none)
//	slug         REQUIRED + IMMUTABLE
func fullResource() *pb.Resource {
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

func newUnary(t *testing.T, opts ...fieldbehavior.Option) grpc.UnaryServerInterceptor {
	t.Helper()

	uni := fieldbehavior.ServerInterceptor(opts...).ServerUnaryInterceptor()
	require.NotNil(t, uni)

	return uni
}

func runUnary(t *testing.T, uni grpc.UnaryServerInterceptor, method string, req, resp any, handlerErr error) (any, any, error) {
	t.Helper()

	info := &grpc.UnaryServerInfo{FullMethod: method}
	var seenReq any
	handler := func(_ context.Context, r any) (any, error) {
		seenReq = r
		return resp, handlerErr
	}

	got, err := uni(context.Background(), req, info, handler)

	return seenReq, got, err
}

func TestServerInterceptor_StripCreateRequest(t *testing.T) {
	t.Parallel()

	uni := newUnary(t)
	req := fullResource()
	seenReq, _, err := runUnary(t, uni, "/x.v1.X/CreateResource", req, &pb.Resource{}, nil)
	require.NoError(t, err)

	got, ok := seenReq.(*pb.Resource)
	require.True(t, ok)
	require.Empty(t, got.GetId(), "IDENTIFIER must be cleared on Create")
	require.Empty(t, got.GetCreateTime(), "OUTPUT_ONLY must be cleared on Create")
	require.Equal(t, "my-resource", got.GetName(), "REQUIRED preserved on Create")
	require.Equal(t, "tenant-1", got.GetTenantId(), "IMMUTABLE preserved on Create")
	require.Equal(t, "secret", got.GetPassword(), "INPUT_ONLY preserved on Create")
	require.Equal(t, "my-slug", got.GetSlug(), "REQUIRED+IMMUTABLE preserved on Create")
}

func TestServerInterceptor_StripUpdateRequest(t *testing.T) {
	t.Parallel()

	uni := newUnary(t)
	req := fullResource()
	seenReq, _, err := runUnary(t, uni, "/x.v1.X/UpdateResource", req, &pb.Resource{}, nil)
	require.NoError(t, err)

	got := seenReq.(*pb.Resource)
	require.Empty(t, got.GetId(), "IDENTIFIER cleared on Update")
	require.Empty(t, got.GetCreateTime(), "OUTPUT_ONLY cleared on Update")
	require.Empty(t, got.GetTenantId(), "IMMUTABLE cleared on Update")
	require.Empty(t, got.GetSlug(), "IMMUTABLE-tagged slug cleared on Update")
	require.Equal(t, "my-resource", got.GetName(), "REQUIRED preserved on Update")
	require.Equal(t, "secret", got.GetPassword(), "INPUT_ONLY preserved on Update")
}

func TestServerInterceptor_StripResponse(t *testing.T) {
	t.Parallel()

	uni := newUnary(t)
	resp := fullResource()
	_, gotResp, err := runUnary(t, uni, "/x.v1.X/GetResource", &pb.Resource{}, resp, nil)
	require.NoError(t, err)

	got := gotResp.(*pb.Resource)
	require.Empty(t, got.GetPassword(), "INPUT_ONLY cleared on response")
	require.Equal(t, "id-1", got.GetId(), "IDENTIFIER preserved on response")
	require.Equal(t, "2026-01-01T00:00:00Z", got.GetCreateTime(), "OUTPUT_ONLY preserved on response")
}

func TestServerInterceptor_HandlerErrorSkipsResponseStrip(t *testing.T) {
	t.Parallel()

	uni := newUnary(t)
	resp := fullResource()
	handlerErr := status.Error(codes.NotFound, "missing")
	_, _, err := runUnary(t, uni, "/x.v1.X/GetResource", &pb.Resource{}, resp, handlerErr)

	require.Error(t, err)
	require.Equal(t, codes.NotFound, status.Code(err))
	// The framework drops the response on handler error before PostCall runs;
	// the invariant we care about — strip not mutating a doomed response —
	// is asserted indirectly: resp.Password is still "secret".
	require.Equal(t, "secret", resp.GetPassword())
}

func TestServerInterceptor_MethodKindOverride(t *testing.T) {
	t.Parallel()

	uni := newUnary(t,
		fieldbehavior.WithMethodKind("/x.v1.X/ImportResource", fieldbehavior.KindCreate),
		fieldbehavior.WithMethodKind("/x.v1.X/GetResource", fieldbehavior.KindSkip),
	)

	req := fullResource()
	seenReq, _, err := runUnary(t, uni, "/x.v1.X/ImportResource", req, &pb.Resource{}, nil)
	require.NoError(t, err)
	require.Empty(t, seenReq.(*pb.Resource).GetId(), "override pushed Import into KindCreate")

	resp := fullResource()
	_, gotResp, err := runUnary(t, uni, "/x.v1.X/GetResource", &pb.Resource{}, resp, nil)
	require.NoError(t, err)
	require.Equal(t, "secret", gotResp.(*pb.Resource).GetPassword(), "KindSkip disables response strip too")
}

func TestServerInterceptor_SkipResponse(t *testing.T) {
	t.Parallel()

	uni := newUnary(t, fieldbehavior.WithSkipResponse())
	resp := fullResource()
	_, gotResp, err := runUnary(t, uni, "/x.v1.X/GetResource", &pb.Resource{}, resp, nil)
	require.NoError(t, err)
	require.Equal(t, "secret", gotResp.(*pb.Resource).GetPassword(), "WithSkipResponse leaves INPUT_ONLY in place")
}

func TestServerInterceptor_IgnoreMethods(t *testing.T) {
	t.Parallel()

	uni := newUnary(t, fieldbehavior.WithIgnoreMethods("/x.v1.X/CreateResource"))
	req := fullResource()
	seenReq, _, err := runUnary(t, uni, "/x.v1.X/CreateResource", req, &pb.Resource{}, nil)
	require.NoError(t, err)
	require.Equal(t, "id-1", seenReq.(*pb.Resource).GetId(), "ignored method is not stripped")
}

func TestServerInterceptor_NonProtoPayload(t *testing.T) {
	t.Parallel()

	uni := newUnary(t)

	type plain struct{ Name string }
	req := &plain{Name: "x"}
	_, gotResp, err := runUnary(t, uni, "/x.v1.X/CreateResource", req, &plain{Name: "y"}, nil)
	require.NoError(t, err)
	require.Equal(t, "y", gotResp.(*plain).Name)
}

func TestServerInterceptor_MaxDepthExceededOnRequest(t *testing.T) {
	t.Parallel()

	// MaxDepth = 0 forces the walker to bail immediately on the first
	// recursion, which is the only deterministic way to surface
	// ErrMaxDepthExceeded from the public API.
	uni := newUnary(t, fieldbehavior.WithMaxStripDepth(0))

	req := &pb.Resource{Profile: &pb.Profile{DisplayName: "x"}}
	_, _, err := runUnary(t, uni, "/x.v1.X/CreateResource", req, &pb.Resource{}, nil)
	require.Error(t, err)
	require.Equal(t, codes.Internal, status.Code(err))
	require.True(t, errors.Is(err, pbfieldbehavior.ErrMaxDepthExceeded), "expected wrapped ErrMaxDepthExceeded, got %v", err)
}
