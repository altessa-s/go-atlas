// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask_test

import (
	"context"
	"io"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/fieldmask"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	pb "github.com/altessa-s/go-atlas/proto/gen/fieldbehaviortest/v1"
)

func TestClassifyMethod(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		method string
		want   fieldmask.Kind
	}{
		{"update", "/x.v1.X/UpdateBucket", fieldmask.KindUpdate},
		{"patch", "/x.v1.X/PatchBucket", fieldmask.KindUpdate},
		{"batch_update", "/x.v1.X/BatchUpdateBuckets", fieldmask.KindUpdate},
		{"get", "/x.v1.X/GetBucket", fieldmask.KindRead},
		{"list", "/x.v1.X/ListBuckets", fieldmask.KindRead},
		{"search", "/x.v1.X/SearchBuckets", fieldmask.KindRead},
		{"batch_get", "/x.v1.X/BatchGetBuckets", fieldmask.KindRead},
		{"create", "/x.v1.X/CreateBucket", fieldmask.KindNone},
		{"delete", "/x.v1.X/DeleteBucket", fieldmask.KindNone},
		{"custom_action", "/x.v1.X/Archive", fieldmask.KindNone},
		{"empty", "", fieldmask.KindNone},
		{"no_slash", "UpdateBucket", fieldmask.KindUpdate},
		{"case_sensitive", "/x.v1.X/updateBucket", fieldmask.KindNone},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, fieldmask.ClassifyMethod(tc.method))
		})
	}
}

func TestKindString(t *testing.T) {
	t.Parallel()

	require.Equal(t, "none", fieldmask.KindNone.String())
	require.Equal(t, "update", fieldmask.KindUpdate.String())
	require.Equal(t, "read", fieldmask.KindRead.String())
	require.Equal(t, "skip", fieldmask.KindSkip.String())
	require.Equal(t, "Kind(99)", fieldmask.Kind(99).String())
}

func newUnary(t *testing.T, opts ...fieldmask.Option) grpc.UnaryServerInterceptor {
	t.Helper()

	uni := fieldmask.ServerInterceptor(opts...).ServerUnaryInterceptor()
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

	got, err := uni(t.Context(), req, info, handler)

	return seenReq, got, err
}

func TestServerInterceptor_UpdateMask_ImmutableRejected(t *testing.T) {
	t.Parallel()

	uni := newUnary(t)
	req := &pb.UpdateResourceRequest{
		Resource:   &pb.Resource{Name: "n", TenantId: "altered"},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"tenant_id"}},
	}

	_, _, err := runUnary(t, uni, "/x.v1.X/UpdateResource", req, &pb.Resource{}, nil)
	require.Error(t, err)
	require.Equal(t, codes.InvalidArgument, status.Code(err))

	st, ok := status.FromError(err)
	require.True(t, ok)

	var found *errdetails.BadRequest
	for _, d := range st.Details() {
		if br, isBR := d.(*errdetails.BadRequest); isBR {
			found = br
			break
		}
	}
	require.NotNil(t, found, "BadRequest detail must be attached")
	require.Len(t, found.GetFieldViolations(), 1)
	require.Equal(t, "tenant_id", found.GetFieldViolations()[0].GetField())
	require.Contains(t, found.GetFieldViolations()[0].GetDescription(), "immutable")
}

func TestServerInterceptor_UpdateMask_OutputOnlyStripped(t *testing.T) {
	t.Parallel()

	uni := newUnary(t)
	req := &pb.UpdateResourceRequest{
		Resource:   &pb.Resource{Name: "n"},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"name", "create_time"}},
	}

	seenReq, _, err := runUnary(t, uni, "/x.v1.X/UpdateResource", req, &pb.Resource{}, nil)
	require.NoError(t, err)

	got, ok := seenReq.(*pb.UpdateResourceRequest)
	require.True(t, ok)

	require.Equal(t, []string{"name"}, got.GetUpdateMask().GetPaths(),
		"OUTPUT_ONLY entry must be removed from the writeback mask")
}

func TestServerInterceptor_UpdateMask_NoMaskPassthrough(t *testing.T) {
	t.Parallel()

	uni := newUnary(t)
	req := &pb.UpdateResourceRequest{
		Resource: &pb.Resource{Name: "n"},
	}

	seenReq, _, err := runUnary(t, uni, "/x.v1.X/UpdateResource", req, &pb.Resource{}, nil)
	require.NoError(t, err)
	require.Equal(t, req, seenReq, "missing update_mask passes through untouched")
}

func TestServerInterceptor_ReadMask_FiltersResponse(t *testing.T) {
	t.Parallel()

	uni := newUnary(t)
	req := &pb.GetResourceRequest{
		Name:     "id-1",
		ReadMask: &fieldmaskpb.FieldMask{Paths: []string{"name"}},
	}
	resp := &pb.Resource{
		Id:          "id-1",
		Name:        "kept",
		TenantId:    "dropped",
		Description: "dropped",
	}

	_, gotResp, err := runUnary(t, uni, "/x.v1.X/GetResource", req, resp, nil)
	require.NoError(t, err)

	got := gotResp.(*pb.Resource)
	require.Equal(t, "kept", got.GetName())
	require.Empty(t, got.GetId())
	require.Empty(t, got.GetTenantId())
	require.Empty(t, got.GetDescription())
}

func TestServerInterceptor_ReadMask_AbsentMaskPassthrough(t *testing.T) {
	t.Parallel()

	uni := newUnary(t)
	req := &pb.GetResourceRequest{Name: "id-1"}
	resp := &pb.Resource{Id: "id-1", Name: "kept", Description: "also kept"}

	_, gotResp, err := runUnary(t, uni, "/x.v1.X/GetResource", req, resp, nil)
	require.NoError(t, err)

	got := gotResp.(*pb.Resource)
	require.Equal(t, "id-1", got.GetId())
	require.Equal(t, "kept", got.GetName())
	require.Equal(t, "also kept", got.GetDescription())
}

func TestServerInterceptor_HandlerErrorSkipsFilter(t *testing.T) {
	t.Parallel()

	uni := newUnary(t)
	req := &pb.GetResourceRequest{
		Name:     "id-1",
		ReadMask: &fieldmaskpb.FieldMask{Paths: []string{"name"}},
	}
	resp := &pb.Resource{Id: "id-1", Description: "leaks"}
	handlerErr := status.Error(codes.NotFound, "missing")

	_, _, err := runUnary(t, uni, "/x.v1.X/GetResource", req, resp, handlerErr)
	require.Error(t, err)
	require.Equal(t, codes.NotFound, status.Code(err))
	require.Equal(t, "leaks", resp.GetDescription(), "PostCall must not run when handler fails")
}

func TestServerInterceptor_SkipReadMask(t *testing.T) {
	t.Parallel()

	uni := newUnary(t, fieldmask.WithSkipReadMask())
	req := &pb.GetResourceRequest{
		Name:     "id-1",
		ReadMask: &fieldmaskpb.FieldMask{Paths: []string{"name"}},
	}
	resp := &pb.Resource{Name: "kept", Description: "also kept"}

	_, gotResp, err := runUnary(t, uni, "/x.v1.X/GetResource", req, resp, nil)
	require.NoError(t, err)
	require.Equal(t, "also kept", gotResp.(*pb.Resource).GetDescription(),
		"WithSkipReadMask must leave the response untouched")
}

func TestServerInterceptor_MethodKindOverride(t *testing.T) {
	t.Parallel()

	uni := newUnary(t,
		fieldmask.WithMethodKind("/x.v1.X/Archive", fieldmask.KindUpdate),
		fieldmask.WithMethodKind("/x.v1.X/UpdateResource", fieldmask.KindSkip),
	)

	t.Run("override to update", func(t *testing.T) {
		t.Parallel()
		req := &pb.UpdateResourceRequest{
			Resource:   &pb.Resource{Name: "n", TenantId: "x"},
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"tenant_id"}},
		}
		_, _, err := runUnary(t, uni, "/x.v1.X/Archive", req, &pb.Resource{}, nil)
		require.Error(t, err)
		require.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("override to skip", func(t *testing.T) {
		t.Parallel()
		req := &pb.UpdateResourceRequest{
			Resource:   &pb.Resource{Name: "n", TenantId: "x"},
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"tenant_id"}},
		}
		_, _, err := runUnary(t, uni, "/x.v1.X/UpdateResource", req, &pb.Resource{}, nil)
		require.NoError(t, err, "KindSkip disables the interceptor for the method")
	})
}

func TestServerInterceptor_IgnoreMethods(t *testing.T) {
	t.Parallel()

	uni := newUnary(t, fieldmask.WithIgnoreMethods("/x.v1.X/UpdateResource"))
	req := &pb.UpdateResourceRequest{
		Resource:   &pb.Resource{Name: "n", TenantId: "x"},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"tenant_id"}},
	}

	_, _, err := runUnary(t, uni, "/x.v1.X/UpdateResource", req, &pb.Resource{}, nil)
	require.NoError(t, err, "ignored method bypasses ApplyUpdateMask")
}

func TestServerInterceptor_NonProtoPayload(t *testing.T) {
	t.Parallel()

	uni := newUnary(t)
	type opaque struct{}
	_, _, err := runUnary(t, uni, "/x.v1.X/UpdateResource", &opaque{}, &opaque{}, nil)
	require.NoError(t, err)
}

func TestServerInterceptor_NonAIPMethodPassthrough(t *testing.T) {
	t.Parallel()

	uni := newUnary(t)
	req := &pb.UpdateResourceRequest{
		Resource:   &pb.Resource{Name: "n", TenantId: "x"},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"tenant_id"}},
	}

	_, _, err := runUnary(t, uni, "/x.v1.X/Archive", req, &pb.Resource{}, nil)
	require.NoError(t, err, "KindNone passes through without ApplyUpdateMask")
}

// maskStream is a minimal grpc.ServerStream: it serves recv on RecvMsg and
// records the messages handed to the transport on SendMsg.
type maskStream struct {
	grpc.ServerStream
	ctx  context.Context
	recv proto.Message
	sent []proto.Message
}

func (s *maskStream) Context() context.Context { return s.ctx }
func (s *maskStream) SendMsg(m any) error {
	msg, _ := m.(proto.Message)
	s.sent = append(s.sent, proto.Clone(msg))
	return nil
}
func (s *maskStream) RecvMsg(m any) error {
	msg, _ := m.(proto.Message)
	proto.Merge(msg, s.recv)
	return nil
}

// The read mask of a streamed Get is applied before each response reaches the
// transport, behind wrappers of earlier interceptors, on a copy of the
// handler's message.
func TestServerStreamInterceptor_ReadMaskBeforeSend(t *testing.T) {
	t.Parallel()

	base := &maskStream{ctx: t.Context(), recv: &pb.GetResourceRequest{
		Name:     "id-1",
		ReadMask: &fieldmaskpb.FieldMask{Paths: []string{"name"}},
	}}
	var ss grpc.ServerStream = interceptors.NewServerWrappedStream(t.Context(), base, nil)
	ss = interceptors.NewServerWrappedStream(t.Context(), ss, interceptors.NoopDriver())

	resp := &pb.Resource{Id: "id-1", Name: "kept", Description: "dropped"}
	stream := fieldmask.ServerInterceptor().ServerStreamInterceptor()
	info := &grpc.StreamServerInfo{FullMethod: "/x.v1.X/GetResource", IsServerStream: true}
	require.NoError(t, stream(nil, ss, info, func(_ any, s grpc.ServerStream) error {
		var req pb.GetResourceRequest
		if err := s.RecvMsg(&req); err != nil {
			return err
		}
		return s.SendMsg(resp)
	}))

	require.Len(t, base.sent, 1)
	sent := base.sent[0].(*pb.Resource)
	require.Equal(t, "kept", sent.GetName())
	require.Empty(t, sent.GetDescription(), "fields outside the read mask must not reach the wire")
	require.Equal(t, "dropped", resp.GetDescription(), "the handler's message must not be mutated")
}

// seqStream serves recv messages in order and records sends; it is safe for a
// concurrent sender and receiver, as gRPC allows on a bidi stream.
type seqStream struct {
	grpc.ServerStream
	ctx  context.Context
	mu   sync.Mutex
	recv []proto.Message
	sent []proto.Message
}

func (s *seqStream) Context() context.Context { return s.ctx }
func (s *seqStream) SendMsg(m any) error {
	msg, _ := m.(proto.Message)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, proto.Clone(msg))
	return nil
}
func (s *seqStream) RecvMsg(m any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.recv) == 0 {
		return io.EOF
	}
	msg, _ := m.(proto.Message)
	proto.Merge(msg, s.recv[0])
	s.recv = s.recv[1:]
	return nil
}

func getRequest(paths ...string) *pb.GetResourceRequest {
	return &pb.GetResourceRequest{Name: "id-1", ReadMask: &fieldmaskpb.FieldMask{Paths: paths}}
}

func runMaskStream(t *testing.T, base *seqStream, handler grpc.StreamHandler) error {
	t.Helper()
	var ss grpc.ServerStream = interceptors.NewServerWrappedStream(t.Context(), base, nil)
	stream := fieldmask.ServerInterceptor().ServerStreamInterceptor()
	info := &grpc.StreamServerInfo{FullMethod: "/x.v1.X/GetResource", IsServerStream: true, IsClientStream: true}
	return stream(nil, ss, info, handler)
}

// Only the first request's read_mask applies: a later request with another
// mask and handler mutation of the received request cannot change it.
func TestServerStreamInterceptor_ReadMaskFirstFrameOwned(t *testing.T) {
	t.Parallel()

	base := &seqStream{ctx: t.Context(), recv: []proto.Message{getRequest("name"), getRequest("description")}}
	resp := &pb.Resource{Name: "kept", Description: "dropped"}
	require.NoError(t, runMaskStream(t, base, func(_ any, s grpc.ServerStream) error {
		var first, second pb.GetResourceRequest
		if err := s.RecvMsg(&first); err != nil {
			return err
		}
		first.ReadMask.Paths = []string{"description"} // handler mutates its request
		if err := s.RecvMsg(&second); err != nil {
			return err
		}
		return s.SendMsg(resp)
	}))

	require.Len(t, base.sent, 1)
	sent := base.sent[0].(*pb.Resource)
	require.Equal(t, "kept", sent.GetName())
	require.Empty(t, sent.GetDescription())
}

// Sends and receives running concurrently on a bidi stream do not race on the
// captured read_mask (run with -race).
func TestServerStreamInterceptor_ConcurrentSendReceive(t *testing.T) {
	t.Parallel()

	const frames = 50
	recv := make([]proto.Message, frames)
	for i := range recv {
		recv[i] = getRequest("name")
	}
	base := &seqStream{ctx: t.Context(), recv: recv}

	require.NoError(t, runMaskStream(t, base, func(_ any, s grpc.ServerStream) error {
		var wg sync.WaitGroup
		wg.Go(func() {
			for {
				var req pb.GetResourceRequest
				if s.RecvMsg(&req) != nil {
					return
				}
			}
		})
		for range frames {
			if err := s.SendMsg(&pb.Resource{Name: "kept", Description: "maybe"}); err != nil {
				return err
			}
		}
		wg.Wait()
		return nil
	}))
	require.Len(t, base.sent, frames)
}

// panicResource panics when reflected, as a broken message would inside
// proto.Clone.
type panicResource struct{ *pb.Resource }

func (panicResource) ProtoReflect() protoreflect.Message { panic("broken message") }

// A panic while cloning the response is converted to Internal and nothing is
// sent.
func TestServerStreamInterceptor_ClonePanic(t *testing.T) {
	t.Parallel()

	base := &seqStream{ctx: t.Context(), recv: []proto.Message{getRequest("name")}}
	var sendErr error
	require.NoError(t, runMaskStream(t, base, func(_ any, s grpc.ServerStream) error {
		var req pb.GetResourceRequest
		if err := s.RecvMsg(&req); err != nil {
			return err
		}
		sendErr = s.SendMsg(panicResource{Resource: &pb.Resource{Name: "x"}})
		return nil
	}))

	require.Equal(t, codes.Internal, status.Code(sendErr))
	require.Empty(t, base.sent)
}
