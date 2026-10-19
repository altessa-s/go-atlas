// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

import (
	"context"
	"errors"
	"reflect"
	"sync/atomic"

	"github.com/altessa-s/go-atlas/core/encoding/keys"
	"github.com/altessa-s/go-atlas/core/text/strings"
	"github.com/altessa-s/go-atlas/data/idempotency"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/driver"
	"github.com/altessa-s/go-atlas/transport/internal/fallback"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/known/anypb"

	sharedmetadata "github.com/altessa-s/go-atlas/transport/grpc/interceptors/metadata"
	internalidem "github.com/altessa-s/go-atlas/transport/internal/idempotency"
	grpcmetadata "google.golang.org/grpc/metadata"
	stdStrings "strings"
)

const interceptorName = "idempotency"

// Name returns the interceptor name used for dependency resolution and chain ordering.
func Name() string { return interceptorName }

// ID is a lightweight [interceptors.Interceptor] reference for this package,
// suitable for passing to exclusion lists.
var ID = interceptors.Ref(interceptorName)

var _ driver.DrivenInterceptor = (*interceptor)(nil)
var _ interceptors.Interceptor = (*interceptor)(nil)

// Idempotency is an alias for [idempotency.Idempotency] from data/idempotency.
type Idempotency = idempotency.Idempotency

// interceptor implements the Idempotency interface.
type interceptor struct {
	interceptors.BaseInterceptor
	i    Idempotency
	opts *options
}

// Dependencies returns interceptors that idempotency requires to run before it.
// Idempotency uses metadata for call information extraction and must run after auth
// so that unauthenticated requests are rejected before idempotency key validation.
func (i *interceptor) Dependencies() []string {
	return []string{sharedmetadata.Name(), auth.Name()}
}

// requestInterceptor handles a single request with its own state
type requestInterceptor struct {
	*interceptor
	meta        *sharedmetadata.CallMetadata
	currentKey  atomic.Value
	lockedKey   string
	lockedState *idempotency.State // populated by AttemptLock; required by Complete
}

// DrivenInterceptor implements the driver.DrivenInterceptor interface.
func (i *interceptor) DrivenInterceptor(ctx context.Context) (driver.Driver, context.Context) {
	// Metadata is guaranteed to be in context by the chain
	meta, _ := sharedmetadata.FromContext(ctx)

	// Create a new instance for each request to avoid race conditions
	ri := &requestInterceptor{
		interceptor: i,
		meta:        meta,
	}
	return ri, ctx
}

// ServerInterceptor creates a new ServerInterceptor that uses the provided storage.
func ServerInterceptor(i Idempotency, opt ...Option) interceptors.ServerInterceptor {
	opts := newOptions(opt...)

	if opts.statusCreator == nil {
		opts.statusCreator = defaultStatusCreator
	}
	if opts.maxReplaySize <= 0 {
		opts.maxReplaySize = DefaultMaxReplaySize
	}

	ic := &interceptor{
		BaseInterceptor: interceptors.NewBaseInterceptorWithFilter(
			interceptorName,
			opts.ignoreMethods,
			opts.ignorePatterns,
			nil, // idempotency interceptor doesn't need logging
		),
		i:    i,
		opts: opts,
	}

	return interceptors.ServerDrivenInterceptor(ic)
}

// defaultStatusCreator creates a gRPC status for an idempotency error and
// attaches an [errdetails.ErrorInfo] detail with a scenario-specific reason code.
func defaultStatusCreator(_ context.Context, scenario ErrorScenario, _ *idempotency.State) *status.Status {
	var (
		st     *status.Status
		reason string
	)
	switch scenario {
	case ErrorIDKMissing:
		st, reason = status.New(codes.InvalidArgument, "Idempotency key is required for this operation"), ReasonIDKMissing
	case ErrorIDKInvalidFormat:
		st, reason = status.New(codes.InvalidArgument, "Idempotency key has an invalid format"), ReasonIDKInvalidFormat
	case ErrorIDKInProgress:
		st, reason = status.New(codes.Aborted, "A request with this idempotency key is currently being processed"), ReasonIDKInProgress
	case ErrorIDKAlreadyUsed:
		st, reason = status.New(codes.FailedPrecondition, "This idempotency key has already been used"), ReasonIDKAlreadyUsed
	case ErrorIDKMismatch:
		st, reason = status.New(codes.InvalidArgument, "This idempotency key was used for a different request"), ReasonIDKMismatch
	case ErrorIDKForbidden:
		st, reason = status.New(codes.PermissionDenied, "This idempotency key belongs to another caller"), ReasonIDKForbidden
	default:
		return status.New(codes.Internal, "Unknown idempotency error")
	}
	if enriched, err := st.WithDetails(&errdetails.ErrorInfo{Reason: reason}); err == nil {
		return enriched
	}
	return st
}

// checkIdempotency performs the idempotency check for a request without
// response replay (streaming calls). It returns an error if the idempotency
// key is not unique.
func (ri *requestInterceptor) checkIdempotency(ctx context.Context, req any) error {
	_, err := ri.check(ctx, req, false)
	return err
}

// check performs the idempotency check for a request. With replay set (a
// unary call) and [WithResponseReplay], a repeat of a completed request
// returns the stored response instead of the already-used error; the caller
// answers with it and skips the handler.
func (ri *requestInterceptor) check(ctx context.Context, req any, replay bool) (proto.Message, error) {
	// Skip check if request is nil or empty
	if req == nil || ri.ShouldIgnore(ri.meta.FullyMethodName) {
		return nil, nil //nolint:nilnil // no replay, no error
	}

	// Fast nil pointer check using type assertion instead of reflection
	if ri.isNilPointer(req) {
		return nil, nil //nolint:nilnil // no replay, no error
	}

	policy := PolicyDefault
	if ri.opts.methodClassifier != nil {
		policy = ri.opts.methodClassifier(ri.meta.FullyMethodName)
	}
	if policy == PolicySkip {
		return nil, nil //nolint:nilnil // no replay, no error
	}

	idempotencyKey := ri.getIdempotencyKey(ctx, req)
	if idempotencyKey == "" {
		if policy.Mandatory(ri.opts.enforceMandatory) {
			return nil, interceptors.NewError(ri.opts.statusCreator(ctx, ErrorIDKMissing, nil), nil)
		}
		return nil, nil //nolint:nilnil // no replay, no error
	}

	if err := ri.opts.keyFormatValidator(idempotencyKey); err != nil {
		return nil, interceptors.NewError(ri.opts.statusCreator(ctx, ErrorIDKInvalidFormat, nil), nil)
	}

	storageKey := ri.buildKey(strings.InternString(ri.meta.FullyMethodName), idempotencyKey)

	locked, state, err := ri.i.AttemptLockWithOpts(ctx, storageKey, idempotency.AttemptLockOpts{
		Fingerprint: ri.requestFingerprint(req),
	})
	outcome, record := internalidem.ClassifyLock(locked, state, err)
	switch outcome {
	case internalidem.LockFingerprintMismatch:
		return nil, interceptors.NewError(ri.opts.statusCreator(ctx, ErrorIDKMismatch, nil), nil)
	case internalidem.LockOwnerMismatch:
		// A ReplayAuthorizer refusal carrying a gRPC status (NotFound to
		// hide the record) is answered with that status.
		if se, ok := errors.AsType[grpcStatusError](err); ok {
			return nil, interceptors.NewError(se.GRPCStatus(), nil)
		}
		return nil, interceptors.NewError(ri.opts.statusCreator(ctx, ErrorIDKForbidden, nil), nil)
	case internalidem.LockStorageError:
		return nil, ri.handleStorageError(err)
	case internalidem.LockNilState:
		return nil, interceptors.NewError(status.New(codes.Internal, "Internal Error: Lock failed but state is nil"), nil)
	case internalidem.LockInProgress:
		_ = grpc.SetHeader(ctx, grpcmetadata.Pairs(ri.opts.idempotencyKeyStatusMetadata, "in_progress")) //nolint:errcheck // Optional metadata
		return nil, interceptors.NewError(ri.opts.statusCreator(ctx, ErrorIDKInProgress, state), nil)
	case internalidem.LockCompleted:
		md := grpcmetadata.Pairs(ri.opts.idempotencyKeyStatusMetadata, "success")
		// Only echo the stored entity id when explicitly enabled. The caller
		// presenting a duplicate key is not verified to be the principal that
		// created the original entity, so returning it by default would leak
		// another principal's entity id to anyone who reuses the key.
		if ri.opts.exposeEntityID && record.EntityID != "" {
			md.Set(ri.opts.idempotencyKeyEntityIdMetadata, record.EntityID)
		}
		_ = grpc.SetHeader(ctx, md) //nolint:errcheck // Optional metadata

		if replay && ri.opts.responseReplay {
			if res := replayedResponse(record, ri.meta.FullyMethodName); res != nil {
				return res, nil
			}
		}
		return nil, interceptors.NewError(ri.opts.statusCreator(ctx, ErrorIDKAlreadyUsed, state), nil)
	case internalidem.LockAcquired:
	}

	// Lock acquired — remember both the key and the *State so PostCall
	// can pass the CAS token back to Complete.
	ri.lockedKey = storageKey
	ri.lockedState = state
	return nil, nil //nolint:nilnil // no replay, no error
}

// replayedResponse returns the response stored in record for a repeat of
// fullMethod, or nil when the record holds none, was written for another
// method (keys are scoped by service only) or before records carried their
// method, the response no longer decodes, or its type is not the method's
// registered response type.
func replayedResponse(record internalidem.Record, fullMethod string) proto.Message {
	if !record.HasGRPCResponse(fullMethod) {
		return nil
	}
	var stored anypb.Any
	if err := proto.Unmarshal(record.Response, &stored); err != nil {
		return nil
	}
	res, err := stored.UnmarshalNew()
	if err != nil {
		return nil
	}
	want, ok := responseType(fullMethod)
	if !ok || res.ProtoReflect().Descriptor().FullName() != want {
		return nil
	}
	return res
}

// responseType returns the response message type of fullMethod
// ("/package.Service/Method") from the global proto registry, false when the
// method is not registered.
func responseType(fullMethod string) (protoreflect.FullName, bool) {
	name := stdStrings.TrimPrefix(fullMethod, "/")
	slash := stdStrings.LastIndexByte(name, '/')
	if slash <= 0 {
		return "", false
	}
	d, err := protoregistry.GlobalFiles.FindDescriptorByName(protoreflect.FullName(name[:slash] + "." + name[slash+1:]))
	if err != nil {
		return "", false
	}
	md, ok := d.(protoreflect.MethodDescriptor)
	if !ok {
		return "", false
	}
	return md.Output().FullName(), true
}

// completionData returns the data stored with a completed key: without
// [WithResponseReplay] the entity ID string (or nil), as before replay
// existed; with it a [internalidem.Record] holding the entity ID and the
// response, when the response is a protobuf message no larger than
// maxReplaySize (otherwise only the entity ID, so a repeat is refused as
// already used).
func (ri *requestInterceptor) completionData(res any) any {
	var entityID string
	if ri.opts.entityIdExtractor != nil {
		entityID = ri.opts.entityIdExtractor(ri.meta.FullyMethodName, res)
	}
	if !ri.opts.responseReplay {
		return internalidem.EntityIDData(entityID)
	}
	record := internalidem.NewRecord(entityID, ri.meta.FullyMethodName)
	record.Response = replayBytes(res, ri.opts.maxReplaySize)
	return record
}

// replayBytes returns res wrapped in a google.protobuf.Any and marshaled, or
// nil when res is not a valid protobuf message, does not marshal, or is
// larger than maxSize.
func replayBytes(res any, maxSize int) []byte {
	msg, ok := res.(proto.Message)
	if !ok || msg == nil || !msg.ProtoReflect().IsValid() {
		return nil
	}
	wrapped, err := anypb.New(msg)
	if err != nil {
		return nil
	}
	raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(wrapped)
	if err != nil || len(raw) > maxSize {
		return nil
	}
	return raw
}

// grpcStatusError is an error carrying a gRPC status, as status.Error makes.
type grpcStatusError interface {
	error
	GRPCStatus() *status.Status
}

// requestFingerprint returns the fingerprint of the method and the
// deterministically marshaled request, or nil when fingerprinting is off or
// req is not a protobuf message that marshals.
func (ri *requestInterceptor) requestFingerprint(req any) []byte {
	if !ri.opts.requestFingerprint {
		return nil
	}
	msg, ok := req.(proto.Message)
	if !ok {
		return nil
	}
	body, err := proto.MarshalOptions{Deterministic: true}.Marshal(msg)
	if err != nil {
		return nil
	}
	return keys.Digest([]byte(ri.meta.FullyMethodName), body)
}

// PostMsgReceive is called after the message is received.
func (ri *requestInterceptor) PostMsgReceive(ctx context.Context, req any, err error) error {
	if ri.meta.IsClient {
		return nil
	} else if err != nil {
		return err
	}

	return ri.checkIdempotency(ctx, req)
}

// PostCall is called after the call is made.
func (ri *requestInterceptor) PostCall(ctx context.Context, res any, err error) error {
	if ri.lockedKey == "" {
		return err
	}

	if err != nil {
		// Release only the lock owned by this failed request.
		_ = ri.i.Release(ctx, ri.lockedKey, ri.lockedState) //nolint:errcheck // Best-effort cleanup
		return err
	}

	// On success, complete the key
	data := ri.completionData(res)

	// Best-effort completion. ErrLockStolen means the lock TTL expired
	// while we were processing and another node took over — our result
	// is no longer authoritative, so silently drop it rather than
	// overwriting the new holder's state.
	_ = ri.i.Complete(ctx, ri.lockedKey, data, ri.lockedState) //nolint:errcheck
	return nil
}

// PreCall is called before the call is made.
func (ri *requestInterceptor) PreCall(ctx context.Context, req any) (any, error) {
	if ri.meta.IsClient {
		return nil, nil //nolint:nilnil
	}

	res, err := ri.check(ctx, req, true)
	if err != nil || res == nil {
		// No replay: an untyped nil lets the handler run.
		return nil, err
	}
	return res, nil
}

// getIdempotencyKey extracts the idempotency key: from the request field
// named by keyField when it holds a non-empty value (such as request_id),
// otherwise from the metadata header.
func (ri *requestInterceptor) getIdempotencyKey(ctx context.Context, req any) string {
	if key, ok := ri.currentKey.Load().(string); ok && key != "" {
		return key
	}

	idempotencyKey := requestFieldKey(req, ri.opts.keyField)
	if idempotencyKey == "" && ri.opts.idempotencyKeyHeader != "" {
		if md, _ := grpcmetadata.FromIncomingContext(ctx); md != nil {
			if vals := md.Get(ri.opts.idempotencyKeyHeader); len(vals) > 0 {
				if v := stdStrings.TrimSpace(vals[0]); v != "" {
					idempotencyKey = v
				}
			}
		}
	}

	if idempotencyKey != "" {
		ri.currentKey.Store(idempotencyKey)
	}

	return idempotencyKey
}

// requestFieldKey returns the trimmed value of the string field named field
// (its proto or JSON name) of req, or "" when field is empty, req is not a
// protobuf message, it has no such singular string field, or the field is
// empty.
func requestFieldKey(req any, field string) string {
	if field == "" {
		return ""
	}
	msg, ok := req.(proto.Message)
	if !ok {
		return ""
	}
	m := msg.ProtoReflect()
	fields := m.Descriptor().Fields()
	fd := fields.ByName(protoreflect.Name(field))
	if fd == nil {
		fd = fields.ByJSONName(field)
	}
	if fd == nil || fd.Kind() != protoreflect.StringKind || fd.Cardinality() == protoreflect.Repeated {
		return ""
	}
	return stdStrings.TrimSpace(m.Get(fd).String())
}

func (ri *requestInterceptor) handleStorageError(err error) error {
	switch ri.opts.fallbackBehavior {
	case fallback.Allow:
		return nil
	case fallback.Deny:
		return interceptors.NewError(status.New(codes.Unavailable, "Service Temporary Unavailable"), nil)
	case fallback.Error:
		if _, ok := status.FromError(err); ok {
			return err
		}
		return interceptors.NewError(status.New(codes.Internal, "Internal Error"), err)
	default:
		return interceptors.NewError(status.New(codes.Internal, "Internal Error"), err)
	}
}

// isNilPointer performs a fast nil pointer check using type assertions
// instead of reflection for better performance (10-100x speedup).
// This method checks for common pointer types used in gRPC and protobuf.
func (ri *requestInterceptor) isNilPointer(req any) bool {
	// Check for nil interface first
	if req == nil {
		return true
	}

	switch v := req.(type) {
	case proto.Message:
		// For protobuf messages, check if the underlying pointer is nil
		// Most protobuf generated structs are pointers
		return v == nil || v.ProtoReflect() == nil
	default:
		vo := reflect.ValueOf(req)
		return vo.Kind() == reflect.Pointer && vo.IsNil()
	}
}

// buildKey constructs a storage key from method name and idempotency key with
// internalidem.BuildStorageKey: the tuple ("transport", service, key) in the
// length-prefixed v2 format, the service being the method without its name.
// Example: idk:v2:9:transport17:users.UserService36:550e8400-e29b-41d4-a716-446655440000
func (ri *requestInterceptor) buildKey(method, key string) string {
	method = stdStrings.TrimPrefix(method, "/") // Remove leading slash if present
	// Extract service name from fully qualified method name (e.g., "users.UserService/CreateUser" -> "users.UserService")
	if idx := stdStrings.LastIndex(method, "/"); idx > 0 {
		method = method[:idx]
	}
	return internalidem.BuildStorageKey(method, key)
}
