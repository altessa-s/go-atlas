// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/idempotency/storages/memory"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/idempotency"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	idempotencydata "github.com/altessa-s/go-atlas/data/idempotency"
	schedulerv1 "github.com/altessa-s/go-atlas/proto/gen/scheduler/v1"
	grpcmetadata "google.golang.org/grpc/metadata"
)

// The test methods must be registered protobuf methods: a response is only
// replayed when its type is the method's response type.
const (
	replayKey    = "0b8a4f7e-6c1d-4e2a-9b3c-5d6e7f8a9b0c"
	replayMethod = schedulerv1.SchedulerService_Get_FullMethodName
)

type ownerKey struct{}

// counter is a unary Get handler that answers with a numbered response and
// counts its calls.
type counter struct{ calls atomic.Int32 }

func (c *counter) handle(_ context.Context, req any) (any, error) {
	n := c.calls.Add(1)
	in, _ := req.(*schedulerv1.TaskGetRequest)
	return &schedulerv1.TaskGetResponse{TaskState: &schedulerv1.TaskState{
		LastRunId: in.GetTaskId() + "#" + string(rune('0'+n)),
	}}, nil
}

// getReq is a Get request for task.
func getReq(task string) proto.Message { return &schedulerv1.TaskGetRequest{TaskId: task} }

func unary(t *testing.T, ic grpc.UnaryServerInterceptor, ctx context.Context, req proto.Message, h grpc.UnaryHandler) (any, error) {
	t.Helper()
	return unaryMethod(t, ic, ctx, replayMethod, req, h)
}

func unaryMethod(t *testing.T, ic grpc.UnaryServerInterceptor, ctx context.Context, method string, req proto.Message,
	h grpc.UnaryHandler,
) (any, error) {
	t.Helper()
	ctx = grpcmetadata.NewIncomingContext(ctx, grpcmetadata.Pairs(idempotency.DefaultIdempotencyKeyHeader, replayKey))
	return ic(ctx, req, &grpc.UnaryServerInfo{FullMethod: method}, h)
}

func newReplayKeeper(opts ...idempotencydata.Option) *idempotencydata.Keeper {
	return idempotencydata.New(memory.New(), append([]idempotencydata.Option{idempotencydata.WithMaxLockDuration(time.Minute)}, opts...)...)
}

// A repeat of a completed request returns the stored response with an OK
// status, without running the handler again.
func TestResponseReplay(t *testing.T) {
	t.Parallel()

	ic := idempotency.ServerInterceptor(newReplayKeeper(), idempotency.WithResponseReplay()).ServerUnaryInterceptor()
	h := &counter{}
	first, err := unary(t, ic, t.Context(), getReq("order"), h.handle)
	require.NoError(t, err)
	second, err := unary(t, ic, t.Context(), getReq("order"), h.handle)
	require.NoError(t, err)
	require.EqualValues(t, 1, h.calls.Load(), "the handler ran once")
	require.True(t, proto.Equal(first.(proto.Message), second.(proto.Message)), "the stored response is replayed: %v", second)

	_, err = unary(t, ic, t.Context(), getReq("other order"), h.handle)
	require.Equal(t, codes.InvalidArgument, status.Code(err), "a different request with the key gets no response")
}

func TestResponseReplayOffByDefault(t *testing.T) {
	t.Parallel()

	ic := idempotency.ServerInterceptor(newReplayKeeper()).ServerUnaryInterceptor()
	h := &counter{}
	_, err := unary(t, ic, t.Context(), getReq("order"), h.handle)
	require.NoError(t, err)
	_, err = unary(t, ic, t.Context(), getReq("order"), h.handle)
	require.Equal(t, codes.FailedPrecondition, status.Code(err))
	require.EqualValues(t, 1, h.calls.Load())
}

// A response larger than the cap is not stored: its repeat is refused as
// already used.
func TestResponseReplaySizeCap(t *testing.T) {
	t.Parallel()

	ic := idempotency.ServerInterceptor(newReplayKeeper(), idempotency.WithResponseReplay(),
		idempotency.WithMaxReplaySize(8)).ServerUnaryInterceptor()
	h := &counter{}
	_, err := unary(t, ic, t.Context(), getReq("a long order description"), h.handle)
	require.NoError(t, err)
	_, err = unary(t, ic, t.Context(), getReq("a long order description"), h.handle)
	require.Equal(t, codes.FailedPrecondition, status.Code(err))
	require.EqualValues(t, 1, h.calls.Load())
}

// A repeat while the first request runs stays ABORTED.
func TestResponseReplayInProgress(t *testing.T) {
	t.Parallel()

	ic := idempotency.ServerInterceptor(newReplayKeeper(), idempotency.WithResponseReplay()).ServerUnaryInterceptor()
	entered, release := make(chan struct{}), make(chan struct{})
	slow := func(ctx context.Context, req any) (any, error) {
		close(entered)
		<-release
		return &schedulerv1.TaskGetResponse{}, nil
	}
	done := make(chan error, 1)
	go func() {
		_, err := unary(t, ic, context.Background(), getReq("order"), slow)
		done <- err
	}()
	<-entered
	_, err := unary(t, ic, t.Context(), getReq("order"), (&counter{}).handle)
	require.Equal(t, codes.Aborted, status.Code(err))
	close(release)
	require.NoError(t, <-done)
}

// A failed call stores nothing: the repeat runs the handler again.
func TestResponseReplayFailureNotStored(t *testing.T) {
	t.Parallel()

	ic := idempotency.ServerInterceptor(newReplayKeeper(), idempotency.WithResponseReplay()).ServerUnaryInterceptor()
	failing := func(context.Context, any) (any, error) { return nil, status.Error(codes.Unavailable, "down") }
	_, err := unary(t, ic, t.Context(), getReq("order"), failing)
	require.Equal(t, codes.Unavailable, status.Code(err))
	h := &counter{}
	_, err = unary(t, ic, t.Context(), getReq("order"), h.handle)
	require.NoError(t, err)
	require.EqualValues(t, 1, h.calls.Load())
}

// Another owner's key is refused before anything of the record, the
// response included, is returned.
func TestResponseReplayOwnerChecked(t *testing.T) {
	t.Parallel()

	k := newReplayKeeper(idempotencydata.WithOwner(func(ctx context.Context) string {
		s, _ := ctx.Value(ownerKey{}).(string)
		return s
	}))
	ic := idempotency.ServerInterceptor(k, idempotency.WithResponseReplay()).ServerUnaryInterceptor()
	h := &counter{}
	_, err := unary(t, ic, context.WithValue(t.Context(), ownerKey{}, "alice"), getReq("order"), h.handle)
	require.NoError(t, err)
	res, err := unary(t, ic, context.WithValue(t.Context(), ownerKey{}, "bob"), getReq("order"), h.handle)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
	require.Nil(t, res)
	res, err = unary(t, ic, context.WithValue(t.Context(), ownerKey{}, "alice"), getReq("order"), h.handle)
	require.NoError(t, err)
	require.NotNil(t, res, "the owner gets the replay")
	require.EqualValues(t, 1, h.calls.Load())
}

// Keys are scoped by service, not method: with fingerprinting off, another
// method of the service presenting the key must not receive the first
// method's response (it is refused as already used, its handler not run).
func TestResponseReplayBoundToMethod(t *testing.T) {
	t.Parallel()

	ic := idempotency.ServerInterceptor(newReplayKeeper(), idempotency.WithResponseReplay(),
		idempotency.WithRequestFingerprint(false)).ServerUnaryInterceptor()
	paused := 0
	pause := func(context.Context, any) (any, error) { paused++; return &schedulerv1.TaskPauseResponse{}, nil }
	resumed := 0
	resume := func(context.Context, any) (any, error) { resumed++; return &schedulerv1.TaskResumeResponse{}, nil }

	_, err := unaryMethod(t, ic, t.Context(), schedulerv1.SchedulerService_Pause_FullMethodName,
		&schedulerv1.TaskPauseRequest{TaskId: "t"}, pause)
	require.NoError(t, err)
	res, err := unaryMethod(t, ic, t.Context(), schedulerv1.SchedulerService_Resume_FullMethodName,
		&schedulerv1.TaskResumeRequest{TaskId: "t"}, resume)
	require.Equal(t, codes.FailedPrecondition, status.Code(err), "another method gets no replay")
	require.Nil(t, res)
	require.Zero(t, resumed)

	res, err = unaryMethod(t, ic, t.Context(), schedulerv1.SchedulerService_Pause_FullMethodName,
		&schedulerv1.TaskPauseRequest{TaskId: "t"}, pause)
	require.NoError(t, err)
	require.IsType(t, &schedulerv1.TaskPauseResponse{}, res, "the same method gets its replay")
	require.Equal(t, 1, paused)
}
