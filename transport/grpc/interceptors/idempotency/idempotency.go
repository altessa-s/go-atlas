// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

import (
	"context"
	"reflect"
	"sync/atomic"

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

	sharedmetadata "github.com/altessa-s/go-atlas/transport/grpc/interceptors/metadata"
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
		st, reason = status.New(codes.InvalidArgument, "Idempotency key must be a valid lowercase UUID v4"), ReasonIDKInvalidFormat
	case ErrorIDKInProgress:
		st, reason = status.New(codes.Aborted, "A request with this idempotency key is currently being processed"), ReasonIDKInProgress
	case ErrorIDKAlreadyUsed:
		st, reason = status.New(codes.FailedPrecondition, "This idempotency key has already been used"), ReasonIDKAlreadyUsed
	default:
		return status.New(codes.Internal, "Unknown idempotency error")
	}
	if enriched, err := st.WithDetails(&errdetails.ErrorInfo{Reason: reason}); err == nil {
		return enriched
	}
	return st
}

// checkIdempotency performs the idempotency check for a request.
// It returns an error if the idempotency key is not unique.
func (ri *requestInterceptor) checkIdempotency(ctx context.Context, req any) error {
	// Skip check if request is nil or empty
	if req == nil || ri.ShouldIgnore(ri.meta.FullyMethodName) {
		return nil
	}

	// Fast nil pointer check using type assertion instead of reflection
	if ri.isNilPointer(req) {
		return nil
	}

	idempotencyKey := ri.getIdempotencyKey(ctx, req)
	if idempotencyKey == "" {
		if ri.opts.enforceMandatory {
			return interceptors.NewError(ri.opts.statusCreator(ctx, ErrorIDKMissing, nil), nil)
		}
		return nil
	}

	if err := ri.opts.keyFormatValidator(idempotencyKey); err != nil {
		return interceptors.NewError(ri.opts.statusCreator(ctx, ErrorIDKInvalidFormat, nil), nil)
	}

	storageKey := ri.buildKey(strings.InternString(ri.meta.FullyMethodName), idempotencyKey)

	locked, state, err := ri.i.AttemptLock(ctx, storageKey)
	if err != nil {
		return ri.handleStorageError(err)
	}

	if !locked {
		if state == nil {
			return interceptors.NewError(status.New(codes.Internal, "Internal Error: Lock failed but state is nil"), nil)
		}

		if state.Status == idempotency.StatusInProgress {
			_ = grpc.SetHeader(ctx, grpcmetadata.Pairs(ri.opts.idempotencyKeyStatusMetadata, "in_progress")) //nolint:errcheck // Optional metadata
			return interceptors.NewError(ri.opts.statusCreator(ctx, ErrorIDKInProgress, state), nil)
		}
		if state.Status == idempotency.StatusSuccess {
			md := grpcmetadata.Pairs(ri.opts.idempotencyKeyStatusMetadata, "success")
			// Only echo the stored entity id when explicitly enabled. The caller
			// presenting a duplicate key is not verified to be the principal that
			// created the original entity, so returning it by default would leak
			// another principal's entity id to anyone who reuses the key.
			if ri.opts.exposeEntityID {
				if strVal, ok := state.Data.(string); ok && strVal != "" {
					md.Set(ri.opts.idempotencyKeyEntityIdMetadata, strVal)
				}
			}
			_ = grpc.SetHeader(ctx, md) //nolint:errcheck // Optional metadata

			return interceptors.NewError(ri.opts.statusCreator(ctx, ErrorIDKAlreadyUsed, state), nil)
		}
	}

	// Lock acquired — remember both the key and the *State so PostCall
	// can pass the CAS token back to Complete.
	ri.lockedKey = storageKey
	ri.lockedState = state
	return nil
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
		// On error, delete the key
		_ = ri.i.Delete(ctx, ri.lockedKey) //nolint:errcheck // Best-effort cleanup
		return err
	}

	// On success, complete the key
	var data any
	if ri.opts.entityIdExtractor != nil {
		if entityID := ri.opts.entityIdExtractor(ri.meta.FullyMethodName, res); entityID != "" {
			data = entityID
		}
	}

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

	return nil, ri.checkIdempotency(ctx, req)
}

// getIdempotencyKey extracts the idempotency key from the request metadata.
func (ri *requestInterceptor) getIdempotencyKey(ctx context.Context, _ any) string {
	if key, ok := ri.currentKey.Load().(string); ok && key != "" {
		return key
	}

	var idempotencyKey string
	if ri.opts.idempotencyKeyHeader != "" {
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

// buildKey constructs a storage key from method name and idempotency key.
// Format idk:{service}:{idempotency_key}
// Example: idk:users.UserService:550e8400-e29b-41d4-a716-446655440000
func (ri *requestInterceptor) buildKey(method, key string) string {
	method = stdStrings.TrimPrefix(method, "/") // Remove leading slash if present
	// Extract service name from fully qualified method name (e.g., "users.UserService/CreateUser" -> "users.UserService")
	if idx := stdStrings.LastIndex(method, "/"); idx > 0 {
		method = method[:idx]
	}
	return "idk:" + method + ":" + key
}
