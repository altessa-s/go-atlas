// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/altessa-s/go-aip/fieldmask/extract"
	"github.com/altessa-s/go-aip/fieldmask/update"

	"github.com/altessa-s/go-atlas/internal/diagnostics"
	"github.com/altessa-s/go-atlas/internal/transport/updatevalidation"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/driver"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/recovery"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/updatemask"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	pbfieldmask "github.com/altessa-s/go-aip/fieldmask"
	sharedrecovery "github.com/altessa-s/go-atlas/internal/transport/recovery"
	sharedmetadata "github.com/altessa-s/go-atlas/transport/grpc/interceptors/metadata"
)

const (
	interceptorName     = "fieldmask"
	updateMaskFieldName = "update_mask"
)

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
	opts        *options
	builtinRead extract.ReadFunc
	// updates applies the update_mask of Update requests.
	updates *updatemask.Normalizer
	// resources caches the resource descriptor of each read method for WithResourceReadMask.
	resources sync.Map
}

// builtinReadExtractor invokes the cached reflection-based read extractor.
func (ri *requestInterceptor) builtinReadExtractor(ctx context.Context, req proto.Message) (*fieldmaskpb.FieldMask, bool) {
	return ri.builtinRead(ctx, req)
}

// Dependencies returns interceptors that fieldmask requires to run before it.
// Recovery covers panics from extractors. Authentication runs before any
// mask is honored.
func (i *interceptor) Dependencies() []string {
	return []string{sharedmetadata.Name(), auth.Name(), recovery.Name()}
}

// requestInterceptor holds per-call state captured at PreCall time.
type requestInterceptor struct {
	*interceptor
	meta *sharedmetadata.CallMetadata
	kind Kind
	plan atomic.Pointer[update.DeferredPlan]
	// readMask is the read_mask of the first request of a KindRead call,
	// published once. A stored snapshot with a nil mask records a request
	// without one. Sends on a stream may run concurrently with receives, so
	// readers load one stable snapshot per response.
	readMask       atomic.Pointer[readMaskSnapshot]
	requestContext context.Context
}

// readMaskSnapshot is the captured read_mask, owned by the interceptor.
type readMaskSnapshot struct {
	mask *fieldmaskpb.FieldMask
	// parsed is the filter tree, built once per call or stream when the
	// mask is captured. Filter only reads it, so concurrent sends share it.
	parsed pbfieldmask.FieldMask
}

// capturedReadMask returns the captured read_mask, or nil when none applies.
func (ri *requestInterceptor) capturedReadMask() *fieldmaskpb.FieldMask {
	snap := ri.readMask.Load()
	if snap == nil || len(snap.mask.GetPaths()) == 0 {
		return nil
	}
	return snap.mask
}

// DrivenInterceptor implements the driver.DrivenInterceptor interface.
func (i *interceptor) DrivenInterceptor(ctx context.Context) (driver.Driver, context.Context) {
	meta, _ := sharedmetadata.FromContext(ctx)

	ri := &requestInterceptor{
		interceptor: i,
		meta:        meta,
		kind:        i.classify(meta.Method()),
	}
	if ri.kind == KindUpdate && meta != nil && meta.IsStream {
		ctx = updatemask.Track(ctx)
	}
	if ri.kind == KindRead {
		ctx = context.WithValue(ctx, readMaskKey{}, ri)
	}

	return ri, ctx
}

// ServerInterceptor returns a server interceptor that wires
// go-aip update.Prepare into Update* methods and [pbfieldmask.FieldMask.Filter] into
// Get/List/Search/BatchGet methods based on the gRPC method name.
func ServerInterceptor(opt ...Option) interceptors.ServerInterceptor {
	opts := newOptions(opt...)

	_, builtinRead := newBuiltinExtractors(extractOptions(opts), opts.metadataReadMaskHeader)

	ic := &interceptor{
		BaseInterceptor: interceptors.NewBaseInterceptorWithFilter(
			interceptorName,
			opts.ignoreMethods,
			opts.ignorePatterns,
			opts.logger,
		),
		opts:        opts,
		builtinRead: builtinRead,
		updates:     newUpdateNormalizer(opts),
	}

	return interceptors.ServerDrivenInterceptor(ic)
}

// MetadataReadMaskHeader returns the effective metadata header configured for
// the built-in read extractor. It returns an empty string for interceptors
// that were not created by [ServerInterceptor].
func MetadataReadMaskHeader(si interceptors.ServerInterceptor) string {
	driven, ok := si.(*interceptors.DrivenServerInterceptor)
	if !ok || driven == nil {
		return ""
	}
	i, ok := driven.Interceptor().(*interceptor)
	if !ok {
		return ""
	}
	return i.opts.metadataReadMaskHeader
}

// classify returns the configured Kind for fullMethod, falling back to the
// AIP-naming heuristic in [ClassifyMethod].
func (i *interceptor) classify(fullMethod string) Kind {
	return methodKind(i.opts, fullMethod)
}

func methodKind(opts *options, fullMethod string) Kind {
	if fullMethod == "" {
		return KindNone
	}

	if k, ok := opts.methodKinds[fullMethod]; ok {
		return k
	}

	return ClassifyMethod(fullMethod)
}

// PreCall applies the update_mask of KindUpdate methods to their resource and
// writes the effective mask back. For KindRead methods it
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

// PreMsgSend runs before every outgoing streaming message is handed to the
// transport and applies the read mask to it. The handler's message is not
// mutated: the mask is applied to a clone, which is sent instead.
func (ri *requestInterceptor) PreMsgSend(ctx context.Context, m any) (out any, err error) {
	if ri.meta != nil && ri.meta.IsClient {
		return m, nil
	}

	if ri.shouldSkip() || ri.opts.skipReadMask {
		return m, nil
	}

	mask := ri.capturedReadMask()
	msg, ok := m.(proto.Message)
	if !ok || ri.kind != KindRead || mask == nil {
		return m, nil
	}

	// Cloning runs outside filterResponse; recover its panics the same way so
	// the response is rejected with Internal instead of crashing SendMsg.
	defer func() {
		if recover() == nil {
			return
		}
		ri.LogError(ctx, "fieldmask read_mask panicked", ri.meta.Method(), nil,
			slog.String("side", "response"),
			slog.String("category", sharedrecovery.PanicCategory),
		)
		out = nil
		err = interceptors.NewError(
			status.New(codes.Internal, "Internal Error"),
			sharedrecovery.ErrPanicRecovered,
		)
	}()

	clone := proto.Clone(msg)
	if err := ri.filterResponse(ctx, clone, mask); err != nil {
		return nil, err
	}

	return clone, nil
}

// PostMsgSent passes the send result through; the read mask is applied before
// sending, in PreMsgSend.
func (ri *requestInterceptor) PostMsgSent(_ context.Context, _ any, err error) error {
	return err
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

// handleRequest dispatches by kind. KindUpdate applies the update_mask and
// writes the effective mask back; KindRead captures the read_mask for use in PostCall.
func (ri *requestInterceptor) handleRequest(ctx context.Context, req any) error {
	msg, ok := req.(proto.Message)
	if !ok {
		return nil
	}

	switch ri.kind {
	case KindUpdate:
		return ri.applyUpdateMask(ctx, msg)
	case KindRead:
		return ri.captureReadMask(ctx, msg)
	case KindNone, KindSkip:
	}

	return nil
}

// handleResponse applies the captured read_mask to the response payload for
// KindRead methods.
func (ri *requestInterceptor) handleResponse(ctx context.Context, resp any) error {
	mask := ri.capturedReadMask()
	if ri.kind != KindRead || mask == nil {
		return nil
	}

	msg, ok := resp.(proto.Message)
	if !ok {
		return nil
	}

	return ri.filterResponse(ctx, msg, mask)
}

// applyUpdateMask applies the update_mask of req to its resource and writes
// the effective mask back (see [updatemask.Normalizer.Normalize]).
func (ri *requestInterceptor) applyUpdateMask(ctx context.Context, req proto.Message) error {
	method := ri.meta.Method()

	ctx, res, err := updatevalidation.Normalize(ctx, ri.updates, req, ri.meta.IsStream)
	if ri.meta.IsStream {
		ri.plan.Store(res.Plan)
	} else {
		ri.requestContext = ctx
	}
	if err != nil {
		if status.Code(err) == codes.Internal {
			ri.LogError(ctx, "fieldmask update_mask apply failed", method, nil,
				slog.String("classification", diagnostics.ErrorClass(err)))
		} else {
			ri.LogDebug(ctx, "fieldmask update_mask rejected", method, slog.String("grpc.code", status.Code(err).String()))
		}

		return err
	}

	if !res.Located {
		ri.LogDebug(ctx, "fieldmask update_mask not present", method,
			slog.String("kind", ri.kind.String()),
		)

		return nil
	}

	ri.LogDebug(ctx, "fieldmask update_mask applied", method,
		slog.Int("cleaned_paths", res.Paths),
	)

	return nil
}

// captureReadMask records the read_mask of the first request, as an owned
// copy, so later requests on a stream and handler mutations of the request
// cannot change it. A missing mask is recorded too and is a silent
// passthrough: responses are not filtered. A malformed mask (AIP-161 path
// grammar, mask limits) is rejected with InvalidArgument before the handler
// runs.
func (ri *requestInterceptor) captureReadMask(ctx context.Context, req proto.Message) error {
	if ri.readMask.Load() != nil {
		return nil
	}

	snap := &readMaskSnapshot{}
	if mask, ok := ri.resolveRead(ctx, req); ok {
		parsed, err := pbfieldmask.FromProtoFieldMask(mask)
		if err != nil {
			return ri.rejectReadMask(ctx)
		}
		if ri.opts.resourceReadMask && !ri.resourcePathsValid(mask.GetPaths()) {
			return ri.rejectReadMask(ctx)
		}
		snap.mask, _ = proto.Clone(mask).(*fieldmaskpb.FieldMask)
		snap.parsed = parsed
	} else {
		ri.LogDebug(ctx, "fieldmask read_mask not present", ri.meta.Method())
	}

	ri.readMask.CompareAndSwap(nil, snap)

	return nil
}

// filterResponse applies the captured read_mask to resp. Panics are
// converted to Internal; the response is rejected rather than allowed
// through half-filtered.
func (ri *requestInterceptor) filterResponse(ctx context.Context, resp proto.Message, rawMask *fieldmaskpb.FieldMask) (err error) {
	method := ri.meta.Method()

	defer func() {
		if recover() == nil {
			return
		}
		ri.LogError(ctx, "fieldmask read_mask panicked", method, nil,
			slog.String("side", "response"),
			slog.String("category", sharedrecovery.PanicCategory),
		)
		err = interceptors.NewError(
			status.New(codes.Internal, "Internal Error"),
			sharedrecovery.ErrPanicRecovered,
		)
	}()

	var msk pbfieldmask.FieldMask
	if snap := ri.readMask.Load(); snap != nil && snap.mask == rawMask && snap.parsed != nil {
		msk = snap.parsed
	} else {
		parsed, parseErr := pbfieldmask.FromProtoFieldMask(rawMask)
		if parseErr != nil {
			return ri.rejectReadMask(ctx)
		}
		msk = parsed
	}
	// A path the response schema does not hold is the client's error
	// (AIP-161); the response is not sent half-filtered.
	filter := msk.Filter
	if ri.opts.resourceReadMask {
		filter = func(resp proto.Message, _ ...pbfieldmask.Option) error { return ri.filterResources(resp, msk) }
	}
	if filterErr := filter(resp); filterErr != nil {
		return ri.rejectReadMask(ctx)
	}

	ri.LogDebug(ctx, "fieldmask read_mask applied", method,
		slog.Int("paths", len(rawMask.GetPaths())),
	)

	return nil
}

const invalidFieldMaskReason = "INVALID_FIELD_MASK"

var invalidReadMaskStatus = func() *status.Status {
	st := status.New(codes.InvalidArgument, invalidReadMaskMessage)
	detailed, err := st.WithDetails(&errdetails.ErrorInfo{Reason: invalidFieldMaskReason})
	if err != nil {
		return st
	}
	return detailed
}()

func (ri *requestInterceptor) rejectReadMask(ctx context.Context) error {
	if st := ri.opts.readMaskErrorStatus; st != nil && st.Code() != codes.OK {
		ri.LogDebug(ctx, "fieldmask read_mask rejected", ri.meta.Method())
		return st.Err()
	}
	ri.LogDebug(ctx, "fieldmask read_mask rejected", ri.meta.Method(),
		slog.String("grpc.error_reason", invalidFieldMaskReason),
	)
	if ri.opts.resourceReadMask {
		return ri.resourceMaskError()
	}
	return invalidReadMaskStatus.Err()
}

// updateMaskName returns the configured update_mask field name.
func updateMaskName(opts *options) string {
	if opts.maskFieldName != "" {
		return opts.maskFieldName
	}
	return updateMaskFieldName
}

// RequestContext attaches a snapshot of this message's deferred update plan.
func (ri *requestInterceptor) RequestContext(ctx context.Context) context.Context {
	if plan := ri.plan.Load(); plan != nil {
		return update.ContextWithPlan(ctx, plan)
	}
	if ri.requestContext != nil {
		return ri.requestContext
	}
	return ctx
}
