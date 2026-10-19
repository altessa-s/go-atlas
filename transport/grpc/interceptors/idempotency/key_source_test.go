// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

import (
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/idempotency"
	"github.com/altessa-s/go-atlas/data/idempotency/storages/memory"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	schedulerv1 "github.com/altessa-s/go-atlas/proto/gen/scheduler/v1"
	sharedmetadata "github.com/altessa-s/go-atlas/transport/grpc/interceptors/metadata"
	internalidem "github.com/altessa-s/go-atlas/transport/internal/idempotency"
	grpcmetadata "google.golang.org/grpc/metadata"
)

// call runs one request of method through a fresh per-request driver of ic,
// with header as the key metadata ("" for none), and completes a held lock.
func call(t *testing.T, ic *interceptor, method, header string, req any) error {
	t.Helper()
	ctx := t.Context()
	if header != "" {
		ctx = grpcmetadata.NewIncomingContext(ctx, grpcmetadata.Pairs(DefaultIdempotencyKeyHeader, header))
	}
	ri := &requestInterceptor{interceptor: ic, meta: &sharedmetadata.CallMetadata{FullyMethodName: method}}
	if err := ri.checkIdempotency(ctx, req); err != nil {
		return err
	}
	return ri.PostCall(ctx, nil, nil)
}

func newKeeper() *idempotency.Keeper {
	return idempotency.New(memory.New(), idempotency.WithMaxLockDuration(time.Minute))
}

func TestRequestFieldKey(t *testing.T) {
	t.Parallel()

	require.Equal(t, "req-1", requestFieldKey(wrapperspb.String(" req-1 "), "value"), "proto name, trimmed")
	require.Empty(t, requestFieldKey(wrapperspb.String("req-1"), ""), "no key field configured")
	require.Empty(t, requestFieldKey(wrapperspb.String("req-1"), "missing"), "no such field")
	require.Empty(t, requestFieldKey(wrapperspb.Int64(7), "value"), "not a string field")
	require.Empty(t, requestFieldKey(&fieldmaskpb.FieldMask{Paths: []string{"a"}}, "paths"), "a repeated field is not a key")
	require.Empty(t, requestFieldKey(struct{}{}, "value"), "not a protobuf message")
}

// The key comes from the request field when it is set (such as request_id)
// and from the header otherwise; the field wins over a different header.
func TestKeyFromRequestField(t *testing.T) {
	t.Parallel()

	ic := newTestInterceptor(newKeeper(), WithKeyField("value"), WithKeyFormatValidator(OpaqueKeyValidator(0)),
		WithRequestFingerprint(false))
	const method = "/svc.v1.Orders/CreateOrder"

	require.NoError(t, call(t, ic, method, "", wrapperspb.String("client-req-1")))
	requireScenario(t, call(t, ic, method, "", wrapperspb.String("client-req-1")), codes.FailedPrecondition, ReasonIDKAlreadyUsed)
	requireScenario(t, call(t, ic, method, "other-header-key", wrapperspb.String("client-req-1")), codes.FailedPrecondition,
		ReasonIDKAlreadyUsed, "the field takes precedence over the header")

	// An empty field falls back to the header.
	require.NoError(t, call(t, ic, method, "header-key", wrapperspb.String("")))
	requireScenario(t, call(t, ic, method, "header-key", wrapperspb.String("")), codes.FailedPrecondition, ReasonIDKAlreadyUsed)

	requireScenario(t, call(t, ic, method, "", wrapperspb.String("has space")), codes.InvalidArgument, ReasonIDKInvalidFormat)
}

func TestKeyFormatValidators(t *testing.T) {
	t.Parallel()

	const method = "/svc.v1.Orders/CreateOrder"
	uuid := newTestInterceptor(newKeeper())
	requireScenario(t, call(t, uuid, method, "client-req-1", wrapperspb.String("x")), codes.InvalidArgument, ReasonIDKInvalidFormat)

	opaque := newTestInterceptor(newKeeper(), WithKeyFormatValidator(OpaqueKeyValidator(12)))
	require.NoError(t, call(t, opaque, method, "client-req-1", wrapperspb.String("x")))
	requireScenario(t, call(t, opaque, method, "client-req-1234", wrapperspb.String("x")), codes.InvalidArgument,
		ReasonIDKInvalidFormat, "longer than the maximum")

	pattern := newTestInterceptor(newKeeper(), WithKeyFormatValidator(PatternKeyValidator(regexp.MustCompile(`req-\d+`), 0)))
	require.NoError(t, call(t, pattern, method, "req-42", wrapperspb.String("x")))
	requireScenario(t, call(t, pattern, method, "req-42x", wrapperspb.String("x")), codes.InvalidArgument,
		ReasonIDKInvalidFormat, "the pattern must match the whole key")
}

func TestMethodClassifier(t *testing.T) {
	t.Parallel()

	ic := newTestInterceptor(newKeeper(), WithMethodClassifier(VerbMethodClassifier(PolicyMandatory)))
	requireScenario(t, call(t, ic, "/svc.v1.Orders/CreateOrder", "", wrapperspb.String("x")), codes.InvalidArgument,
		ReasonIDKMissing, "Create requires a key")
	requireScenario(t, call(t, ic, "/svc.v1.Orders/CancelOrder", "", wrapperspb.String("x")), codes.InvalidArgument,
		ReasonIDKMissing, "a custom verb requires a key")
	require.NoError(t, call(t, ic, "/svc.v1.Orders/GetOrder", "", wrapperspb.String("x")), "Get is skipped")
	require.NoError(t, call(t, ic, "/svc.v1.Orders/UpdateOrder", "not-a-uuid", wrapperspb.String("x")),
		"a skipped method does not even validate the key")

	optional := newTestInterceptor(newKeeper(), WithEnforceMandatory(true),
		WithMethodClassifier(func(string) Policy { return PolicyOptional }))
	require.NoError(t, call(t, optional, "/svc.v1.Orders/CreateOrder", "", wrapperspb.String("x")),
		"an optional method overrides enforceMandatory")

	enforced := newTestInterceptor(newKeeper(), WithEnforceMandatory(true),
		WithMethodClassifier(func(string) Policy { return PolicyDefault }))
	requireScenario(t, call(t, enforced, "/svc.v1.Orders/CreateOrder", "", wrapperspb.String("x")), codes.InvalidArgument,
		ReasonIDKMissing, "the default policy follows enforceMandatory")
}

// Blank skip verbs are dropped: they would prefix every method name and let
// a mandatory method through without a key.
func TestVerbMethodClassifierIgnoresBlankVerbs(t *testing.T) {
	t.Parallel()

	ic := newTestInterceptor(newKeeper(),
		WithMethodClassifier(VerbMethodClassifier(PolicyMandatory, WithSkipVerbs("Get", " ", ""))))
	requireScenario(t, call(t, ic, "/svc.v1.Orders/CreateOrder", "", wrapperspb.String("x")), codes.InvalidArgument,
		ReasonIDKMissing, "a blank skip verb must not bypass PolicyMandatory")
	require.NoError(t, call(t, ic, "/svc.v1.Orders/GetOrder", "", wrapperspb.String("x")), "Get is still skipped")

	blanks := newTestInterceptor(newKeeper(), WithMethodClassifier(VerbMethodClassifier(PolicyMandatory, WithSkipVerbs(" "))))
	requireScenario(t, call(t, blanks, "/svc.v1.Orders/GetOrder", "", wrapperspb.String("x")), codes.InvalidArgument,
		ReasonIDKMissing, "only blank verbs: nothing is skipped")
}

// An ignored method is never classified: the ignore lists run first.
func TestMethodClassifierRunsAfterIgnoreLists(t *testing.T) {
	t.Parallel()

	calls := 0
	ic := newTestInterceptor(newKeeper(), WithMethodClassifier(func(string) Policy { calls++; return PolicyMandatory }))
	ic.BaseInterceptor = interceptors.NewBaseInterceptorWithFilter(interceptorName, []string{"/svc.v1.Orders/CreateOrder"}, nil, nil)
	require.NoError(t, call(t, ic, "/svc.v1.Orders/CreateOrder", "", wrapperspb.String("x")))
	require.Zero(t, calls, "an ignored method is never classified")
}

// completeKey stores data as the completed record of the test key of method
// for req, as a successful call would.
func completeKey(t *testing.T, k *idempotency.Keeper, ic *interceptor, method string, req proto.Message, data any) {
	t.Helper()
	ri := &requestInterceptor{interceptor: ic, meta: &sharedmetadata.CallMetadata{FullyMethodName: method}}
	storageKey := ri.buildKey(method, testKey)
	ok, state, err := k.AttemptLockWithOpts(t.Context(), storageKey, idempotency.AttemptLockOpts{Fingerprint: ri.requestFingerprint(req)})
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, k.Complete(t.Context(), storageKey, data, state))
}

// Replay applies to unary calls only, and only to records holding a response
// of the same method whose type is the method's response type: a stream
// repeat, a record written before replay (a bare entity ID) or before records
// carried their method, a record of another method, an unknown message type
// and a response of another type are refused as already used.
func TestResponseReplayOnlyForDecodableUnaryRecords(t *testing.T) {
	t.Parallel()

	const method = schedulerv1.SchedulerService_Get_FullMethodName
	ctx := grpcmetadata.NewIncomingContext(t.Context(), grpcmetadata.Pairs(DefaultIdempotencyKeyHeader, testKey))
	req := &schedulerv1.TaskGetRequest{TaskId: "t"}
	created := &schedulerv1.TaskGetResponse{TaskState: &schedulerv1.TaskState{LastRunId: "run-1"}}
	newRI := func(ic *interceptor) *requestInterceptor {
		return &requestInterceptor{interceptor: ic, meta: &sharedmetadata.CallMetadata{FullyMethodName: method}}
	}
	refused := func(t *testing.T, data any, why string) {
		t.Helper()
		k := newKeeper()
		ic := newTestInterceptor(k, WithResponseReplay())
		completeKey(t, k, ic, method, req, data)
		_, err := newRI(ic).check(ctx, req, true)
		requireScenario(t, err, codes.FailedPrecondition, ReasonIDKAlreadyUsed, why)
	}

	k := newKeeper()
	ic := newTestInterceptor(k, WithResponseReplay())
	completeKey(t, k, ic, method, req, internalidem.Record{Binding: method, Response: replayBytes(created, DefaultMaxReplaySize)})
	res, err := newRI(ic).check(ctx, req, true)
	require.NoError(t, err)
	require.True(t, proto.Equal(created, res))
	requireScenario(t, newRI(ic).checkIdempotency(ctx, req), codes.FailedPrecondition, ReasonIDKAlreadyUsed, "streams never replay")

	refused(t, "entity-1", "a bare entity ID holds no response")
	refused(t, internalidem.Record{Response: replayBytes(created, DefaultMaxReplaySize)}, "a record without a binding")
	refused(t, internalidem.Record{Binding: schedulerv1.SchedulerService_List_FullMethodName,
		Response: replayBytes(created, DefaultMaxReplaySize)}, "a record of another method")
	stale, err := proto.Marshal(&anypb.Any{TypeUrl: "type.googleapis.com/gone.v1.Removed", Value: []byte{1}})
	require.NoError(t, err)
	refused(t, internalidem.Record{Binding: method, Response: stale}, "an unknown response type")
	refused(t, internalidem.Record{Binding: method, Response: replayBytes(&schedulerv1.TaskPauseResponse{}, DefaultMaxReplaySize)},
		"a response that is not the method's response type")
}

func TestResponseType(t *testing.T) {
	t.Parallel()

	got, ok := responseType(schedulerv1.SchedulerService_Get_FullMethodName)
	require.True(t, ok)
	require.EqualValues(t, "io.altessa.grpc.scheduler.v1.TaskGetResponse", got)
	for _, m := range []string{"/svc.v1.Orders/CreateOrder", "/io.altessa.grpc.scheduler.v1.SchedulerService/Nope", "nomethod", "/x"} {
		_, ok := responseType(m)
		require.False(t, ok, m)
	}
}

func TestCompletionData(t *testing.T) {
	t.Parallel()

	extract := WithEntityIDExtractor(func(string, any) string { return "entity-1" })
	meta := &sharedmetadata.CallMetadata{FullyMethodName: "/a/B"}
	plain := &requestInterceptor{interceptor: newTestInterceptor(nil, extract), meta: meta}
	require.Equal(t, "entity-1", plain.completionData(wrapperspb.String("x")), "without replay the entity ID string is stored, as before")
	require.Nil(t, (&requestInterceptor{interceptor: newTestInterceptor(nil), meta: meta}).completionData(wrapperspb.String("x")))

	replay := &requestInterceptor{interceptor: newTestInterceptor(nil, extract, WithResponseReplay()), meta: meta}
	record, ok := replay.completionData(wrapperspb.String("x")).(internalidem.Record)
	require.True(t, ok)
	require.Equal(t, "entity-1", record.EntityID)
	require.Equal(t, "/a/B", record.Binding, "the record is bound to the method")
	require.True(t, record.HasGRPCResponse("/a/B"))
	require.False(t, record.HasGRPCResponse("/a/C"))
	require.Empty(t, replay.completionData(struct{}{}).(internalidem.Record).Response, "not a protobuf message")
}
