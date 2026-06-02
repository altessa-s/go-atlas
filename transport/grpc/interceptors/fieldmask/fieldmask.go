// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask

import (
	"context"
	"errors"
	"log/slog"

	"github.com/altessa-s/go-atlas/core/runtime/panics"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/driver"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	pbfieldmask "github.com/altessa-s/go-atlas/domain/proto/fieldmask"
	sharedmetadata "github.com/altessa-s/go-atlas/transport/grpc/interceptors/metadata"
)

const interceptorName = "fieldmask"

// Name returns the interceptor name used for dependency resolution and chain
// ordering.
func Name() string { return interceptorName }

// ID is a lightweight [interceptors.Interceptor] reference for this package,
// suitable for passing to exclusion lists.
var ID = interceptors.Ref(interceptorName)

var (
	_ driver.DrivenInterceptor = (*interceptor)(nil)
	_ driver.DriverStream      = (*requestInterceptor)(nil)
	_ interceptors.Interceptor = (*interceptor)(nil)
)

// interceptor implements the fieldmask gRPC interceptor.
type interceptor struct {
	interceptors.BaseInterceptor
	opts          *options
	extractOpts   []pbfieldmask.ExtractOption
	builtinUpdate pbfieldmask.UpdateExtractorFunc
	builtinRead   pbfieldmask.ReadExtractorFunc
}

// builtinUpdateExtractor invokes the cached reflection-based update
// extractor on the current request. Used as the bottom-of-stack fallback
// after per-method and global-default overrides miss.
func (ri *requestInterceptor) builtinUpdateExtractor(
	ctx context.Context,
	req proto.Message,
) (*fieldmaskpb.FieldMask, proto.Message, func(*fieldmaskpb.FieldMask), bool) {
	return ri.builtinUpdate(ctx, req)
}

// builtinReadExtractor invokes the cached reflection-based read extractor.
func (ri *requestInterceptor) builtinReadExtractor(ctx context.Context, req proto.Message) (*fieldmaskpb.FieldMask, bool) {
	return ri.builtinRead(ctx, req)
}

// Dependencies returns interceptors that fieldmask requires to run before it.
// Metadata is needed for the method name; auth must reject unauthenticated
// requests before any mask is honored.
func (i *interceptor) Dependencies() []string {
	return []string{sharedmetadata.Name(), auth.Name()}
}

// requestInterceptor holds per-call state captured at PreCall time.
type requestInterceptor struct {
	*interceptor
	meta     *sharedmetadata.CallMetadata
	kind     Kind
	readMask *fieldmaskpb.FieldMask // captured at PreCall for KindRead methods
}

// DrivenInterceptor implements the driver.DrivenInterceptor interface.
func (i *interceptor) DrivenInterceptor(ctx context.Context) (driver.Driver, context.Context) {
	meta, _ := sharedmetadata.FromContext(ctx)

	ri := &requestInterceptor{
		interceptor: i,
		meta:        meta,
		kind:        i.classify(meta.Method()),
	}

	return ri, ctx
}

// ServerInterceptor returns a server interceptor that wires
// [pbfieldmask.FieldMask.ApplyUpdateMask] into Update* methods and
// [pbfieldmask.FieldMask.Filter] into Get/List/Search/BatchGet methods based
// on the gRPC method name.
func ServerInterceptor(opt ...Option) interceptors.ServerInterceptor {
	opts := newOptions(opt...)

	var extractOpts []pbfieldmask.ExtractOption
	if opts.maskFieldName != "" {
		extractOpts = append(extractOpts, pbfieldmask.WithMaskField(opts.maskFieldName))
	}
	if opts.resourceFieldName != "" {
		extractOpts = append(extractOpts, pbfieldmask.WithResourceField(opts.resourceFieldName))
	}

	builtinUpdate, builtinRead := newBuiltinExtractors(extractOpts, opts.metadataReadMaskHeader)

	ic := &interceptor{
		BaseInterceptor: interceptors.NewBaseInterceptorWithFilter(
			interceptorName,
			opts.ignoreMethods,
			opts.ignorePatterns,
			opts.logger,
		),
		opts:          opts,
		extractOpts:   extractOpts,
		builtinUpdate: builtinUpdate,
		builtinRead:   builtinRead,
	}

	return interceptors.ServerDrivenInterceptor(ic)
}

// classify returns the configured Kind for fullMethod, falling back to the
// AIP-naming heuristic in [ClassifyMethod].
func (i *interceptor) classify(fullMethod string) Kind {
	if fullMethod == "" {
		return KindNone
	}

	if k, ok := i.opts.methodKinds[fullMethod]; ok {
		return k
	}

	return ClassifyMethod(fullMethod)
}

// PreCall extracts the mask + resource for KindUpdate methods, applies
// ApplyUpdateMask, and writes the cleaned mask back. For KindRead methods it
// captures the read_mask for use in PostCall. Other kinds short-circuit.
func (ri *requestInterceptor) PreCall(ctx context.Context, req any) (any, error) {
	if ri.meta != nil && ri.meta.IsClient {
		return nil, nil //nolint:nilnil // Client-side calls are not handled here.
	}

	if ri.shouldSkip() {
		return nil, nil //nolint:nilnil // Filter / KindSkip override.
	}

	if err := ri.handleRequest(ctx, req); err != nil {
		return nil, err
	}

	return nil, nil //nolint:nilnil // Proceed to handler.
}

// PostCall applies Filter to the response for KindRead methods. A handler
// error short-circuits — the response body is not exposed to the wire.
func (ri *requestInterceptor) PostCall(ctx context.Context, resp any, err error) error {
	if err != nil {
		return err
	}

	if ri.meta != nil && ri.meta.IsClient {
		return nil
	}

	if ri.shouldSkip() || ri.opts.skipReadMask {
		return nil
	}

	return ri.handleResponse(ctx, resp)
}

// PostMsgReceive runs on every incoming streaming message. The classification
// is computed once at stream start and applied per frame.
func (ri *requestInterceptor) PostMsgReceive(ctx context.Context, req any, err error) error {
	if err != nil {
		return err
	}

	if ri.meta != nil && ri.meta.IsClient {
		return nil
	}

	if ri.shouldSkip() {
		return nil
	}

	return ri.handleRequest(ctx, req)
}

// PostMsgSent runs on every outgoing streaming message.
func (ri *requestInterceptor) PostMsgSent(ctx context.Context, resp any, err error) error {
	if err != nil {
		return err
	}

	if ri.meta != nil && ri.meta.IsClient {
		return nil
	}

	if ri.shouldSkip() || ri.opts.skipReadMask {
		return nil
	}

	return ri.handleResponse(ctx, resp)
}

// shouldSkip reports whether the current method should bypass the interceptor.
func (ri *requestInterceptor) shouldSkip() bool {
	if ri.kind == KindSkip || ri.kind == KindNone {
		return true
	}

	if ri.meta == nil {
		return false
	}

	return ri.ShouldIgnore(ri.meta.FullyMethodName)
}

// handleRequest dispatches by kind. KindUpdate runs ApplyUpdateMask and
// writes back; KindRead captures the read_mask for use in PostCall.
func (ri *requestInterceptor) handleRequest(ctx context.Context, req any) error {
	msg, ok := req.(proto.Message)
	if !ok {
		return nil
	}

	switch ri.kind {
	case KindUpdate:
		return ri.applyUpdateMask(ctx, msg)
	case KindRead:
		ri.captureReadMask(ctx, msg)
		return nil
	case KindNone, KindSkip:
	}

	return nil
}

// handleResponse applies the captured read_mask to the response payload for
// KindRead methods.
func (ri *requestInterceptor) handleResponse(ctx context.Context, resp any) error {
	if ri.kind != KindRead || ri.readMask == nil || len(ri.readMask.GetPaths()) == 0 {
		return nil
	}

	msg, ok := resp.(proto.Message)
	if !ok {
		return nil
	}

	return ri.filterResponse(ctx, msg, ri.readMask)
}

// applyUpdateMask runs ApplyUpdateMask against the request and writes the
// cleaned mask back. Returns a converted gRPC status error on violation.
func (ri *requestInterceptor) applyUpdateMask(ctx context.Context, req proto.Message) (err error) {
	method := ri.meta.Method()

	defer panics.HandleWithOpts(ctx, panics.NewHandleOpts().SetReallyPanic(false), func(_ context.Context, r any) {
		ri.LogError(ctx, "fieldmask update_mask panicked", method, nil,
			slog.String("side", "request"),
			slog.String("kind", ri.kind.String()),
			slog.Any("panic", r),
		)
		err = interceptors.NewError(
			status.New(codes.Internal, "Internal Error"),
			nil,
		)
	})

	rawMask, resource, writeback, ok := ri.resolveUpdate(ctx, req)
	if !ok {
		ri.LogDebug(ctx, "fieldmask update_mask not present", method,
			slog.String("kind", ri.kind.String()),
		)
		return nil
	}

	msk := pbfieldmask.FromProtoFieldMask(rawMask)

	if applyErr := msk.ApplyUpdateMask(resource); applyErr != nil {
		return ri.convertUpdateError(ctx, method, applyErr)
	}

	cleaned := msk.ToProtoFieldMask()
	if writeback != nil {
		writeback(cleaned)
	}

	ri.LogDebug(ctx, "fieldmask update_mask applied", method,
		slog.Int("cleaned_paths", len(cleaned.GetPaths())),
	)

	return nil
}

// captureReadMask records the request's read_mask for later use in PostCall.
// Missing mask is a silent passthrough — the handler will see the request
// as-is and PostCall will skip filtering.
func (ri *requestInterceptor) captureReadMask(ctx context.Context, req proto.Message) {
	mask, ok := ri.resolveRead(ctx, req)
	if !ok {
		ri.LogDebug(ctx, "fieldmask read_mask not present", ri.meta.Method())
		return
	}

	ri.readMask = mask
}

// filterResponse applies the captured read_mask to resp. Panics are
// converted to Internal; the response is rejected rather than allowed
// through half-filtered.
func (ri *requestInterceptor) filterResponse(ctx context.Context, resp proto.Message, rawMask *fieldmaskpb.FieldMask) (err error) {
	method := ri.meta.Method()

	defer panics.HandleWithOpts(ctx, panics.NewHandleOpts().SetReallyPanic(false), func(_ context.Context, r any) {
		ri.LogError(ctx, "fieldmask read_mask panicked", method, nil,
			slog.String("side", "response"),
			slog.Any("panic", r),
		)
		err = interceptors.NewError(
			status.New(codes.Internal, "Internal Error"),
			nil,
		)
	})

	msk := pbfieldmask.FromProtoFieldMask(rawMask)
	msk.Filter(resp)

	ri.LogDebug(ctx, "fieldmask read_mask applied", method,
		slog.Int("paths", len(rawMask.GetPaths())),
	)

	return nil
}

// convertUpdateError maps ApplyUpdateMask errors to gRPC status responses.
// BehaviorViolationError becomes InvalidArgument + google.rpc.BadRequest;
// ValidationError becomes InvalidArgument; everything else becomes Internal.
func (ri *requestInterceptor) convertUpdateError(ctx context.Context, method string, applyErr error) error {
	var behaviorErr *pbfieldmask.BehaviorViolationError
	if errors.As(applyErr, &behaviorErr) {
		ri.LogDebug(ctx, "fieldmask update_mask behavior violation", method,
			slog.Int("violations", len(behaviorErr.Violations)),
		)
		return interceptors.NewError(buildBehaviorStatus(behaviorErr), applyErr)
	}

	var validationErr *pbfieldmask.ValidationError
	if errors.As(applyErr, &validationErr) {
		ri.LogDebug(ctx, "fieldmask update_mask validation error", method,
			slog.String("path", validationErr.Path),
		)
		st := status.New(codes.InvalidArgument, applyErr.Error())
		return interceptors.NewError(st, applyErr)
	}

	ri.LogError(ctx, "fieldmask update_mask apply failed", method, applyErr)
	return interceptors.NewError(
		status.New(codes.Internal, "Internal Error"),
		applyErr,
	)
}

// buildBehaviorStatus produces an InvalidArgument status carrying a
// google.rpc.BadRequest detail with one FieldViolation per offending field.
func buildBehaviorStatus(err *pbfieldmask.BehaviorViolationError) *status.Status {
	violations := make([]*errdetails.BadRequest_FieldViolation, 0, len(err.Violations))
	for _, v := range err.Violations {
		violations = append(violations, &errdetails.BadRequest_FieldViolation{
			Field:       v.Field,
			Description: v.Description,
		})
	}

	st := status.New(codes.InvalidArgument, "field behavior violation")
	if detailed, derr := st.WithDetails(&errdetails.BadRequest{
		FieldViolations: violations,
	}); derr == nil {
		return detailed
	}

	return st
}
