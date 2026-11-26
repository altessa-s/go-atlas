// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldbehavior

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"

	"github.com/altessa-s/go-aip/fieldmask/update"
	"github.com/altessa-s/proto-gen-go/io/altessa/badrequest/v1"

	"github.com/altessa-s/go-atlas-core/types/ptr"
	"github.com/altessa-s/go-atlas/internal/diagnostics"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/driver"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/recovery"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/updatemask"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	pbfieldbehavior "github.com/altessa-s/go-aip/fieldbehavior"
	sharedrecovery "github.com/altessa-s/go-atlas/internal/transport/recovery"
	sharedmetadata "github.com/altessa-s/go-atlas/transport/grpc/interceptors/metadata"
)

const interceptorName = "fieldbehavior"

// Name returns the interceptor name used for dependency resolution and chain
// ordering.
func Name() string { return interceptorName }

// ID is a lightweight [interceptors.Interceptor] reference for this package,
// suitable for passing to exclusion lists.
var ID = interceptors.Ref(interceptorName)

var (
	_ interceptors.ServerInterceptor = (*interceptor)(nil)
	_ driver.DrivenInterceptor       = (*interceptor)(nil)
	_ driver.DriverUnaryResponse     = (*requestInterceptor)(nil)
	_ driver.DriverStream            = (*requestInterceptor)(nil)
	_ driver.DriverStreamPreSend     = (*requestInterceptor)(nil)
	_ interceptors.Interceptor       = (*interceptor)(nil)
)

// interceptor implements the fieldbehavior gRPC interceptor.
type interceptor struct {
	interceptors.BaseInterceptor
	opts       *options
	stripOpts  []pbfieldbehavior.Option
	detectOpts []pbfieldbehavior.Option
	// updates applies the update_mask of Update requests.
	updates *updatemask.Normalizer
}

// Dependencies returns interceptors that fieldbehavior requires to run before
// it. Authentication rejects unauthenticated calls before payload mutation.
// Recovery covers panics during mutation.
func (i *interceptor) Dependencies() []string {
	return []string{sharedmetadata.Name(), auth.Name(), recovery.Name()}
}

// requestInterceptor holds per-call state captured at PreCall time.
type requestInterceptor struct {
	*interceptor
	meta *sharedmetadata.CallMetadata
	kind Kind
	plan atomic.Pointer[update.DeferredPlan]
}

// DrivenInterceptor implements the driver.DrivenInterceptor interface.
func (i *interceptor) DrivenInterceptor(ctx context.Context) (driver.Driver, context.Context) {
	meta, _ := sharedmetadata.FromContext(ctx)

	ri := &requestInterceptor{
		interceptor: i,
		meta:        meta,
		kind:        i.classify(meta.Method()),
	}
	if ri.kind == KindUpdate {
		ctx = updatemask.Track(ctx)
	}

	return ri, ctx
}

// ServerInterceptor returns a server interceptor that strips field_behavior
// annotations from gRPC requests and responses based on the method name.
func ServerInterceptor(opt ...Option) interceptors.ServerInterceptor {
	opts := newOptions(opt...)

	maxDepth := pbfieldbehavior.WithMaxDepth(opts.maxStripDepth)

	if opts.updateNormalizer == nil {
		opts.updateNormalizer = updatemask.New()
	}

	ic := &interceptor{
		BaseInterceptor: interceptors.NewBaseInterceptorWithFilter(
			interceptorName,
			opts.ignoreMethods,
			opts.ignorePatterns,
			opts.logger,
		),
		opts:       opts,
		stripOpts:  []pbfieldbehavior.Option{maxDepth},
		detectOpts: []pbfieldbehavior.Option{maxDepth, pbfieldbehavior.WithStrict(true)},
		updates:    opts.updateNormalizer,
	}

	return ic
}

// ServerUnaryInterceptor returns a unary server interceptor that strips
// request fields and removes populated INPUT_ONLY fields from a response copy.
func (i *interceptor) ServerUnaryInterceptor() grpc.UnaryServerInterceptor {
	return interceptors.ServerDrivenInterceptor(i).ServerUnaryInterceptor()
}

// ServerStreamInterceptor returns a streaming server interceptor.
func (i *interceptor) ServerStreamInterceptor() grpc.StreamServerInterceptor {
	return interceptors.ServerDrivenInterceptor(i).ServerStreamInterceptor()
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

// PreCall runs the request-side Strip before the handler executes.
func (ri *requestInterceptor) PreCall(ctx context.Context, req any) (any, error) {
	if ri.meta != nil && ri.meta.IsClient {
		return nil, nil //nolint:nilnil // Client-side calls are not stripped here.
	}

	if ri.shouldSkip() {
		return nil, nil //nolint:nilnil // Filter / KindSkip override.
	}

	if err := ri.stripRequest(ctx, req); err != nil {
		return nil, err
	}

	return nil, nil //nolint:nilnil // Proceed to handler.
}

// PostCall passes through the handler error. Response replacement is handled
// by PostCallResponse for unary calls and PreMsgSend for streams.
func (ri *requestInterceptor) PostCall(_ context.Context, _ any, err error) error {
	return err
}

// PostCallResponse sanitizes the unary response returned by the handler.
func (ri *requestInterceptor) PostCallResponse(ctx context.Context, resp any) (any, error) {
	return ri.stripResponse(ctx, resp)
}

// PostMsgReceive runs on every incoming streaming message and applies the
// request-side Strip mirroring the unary PreCall path.
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

	return ri.stripRequest(ctx, req)
}

// PreMsgSend runs before every outgoing streaming message is handed to the
// transport and strips INPUT_ONLY fields from it. The handler's message is not
// mutated: when it holds a populated INPUT_ONLY field, a clone is stripped and
// sent instead.
func (ri *requestInterceptor) PreMsgSend(ctx context.Context, m any) (any, error) {
	return ri.stripResponse(ctx, m)
}

// PostMsgSent passes the send result through; responses are stripped before
// sending, in PreMsgSend.
func (ri *requestInterceptor) PostMsgSent(_ context.Context, _ any, err error) error {
	return err
}

// shouldSkip reports whether the current method is filtered out via
// ignoreMethods/ignorePatterns or via a KindSkip override.
func (ri *requestInterceptor) shouldSkip() bool {
	if ri.kind == KindSkip {
		return true
	}

	if ri.meta == nil {
		return false
	}

	return ri.ShouldIgnore(ri.meta.FullyMethodName)
}

// stripRequest applies the kind-appropriate Strip* function to req. Returns a
// gRPC status error on traversal failures.
func (ri *requestInterceptor) stripRequest(ctx context.Context, req any) error {
	msg, ok := req.(proto.Message)
	if !ok {
		return nil
	}

	switch ri.kind {
	case KindCreate:
		return ri.runStrip(ctx, msg, pbfieldbehavior.StripCreate, ri.kind.String(), "request")
	case KindUpdate:
		return ri.stripUpdateRequest(ctx, msg)
	case KindNone, KindSkip:
	}

	return nil
}

// stripResponse checks the response without mutating it and copies it only
// when there are populated INPUT_ONLY fields to remove.
func (ri *requestInterceptor) stripResponse(ctx context.Context, resp any) (out any, err error) {
	if (ri.meta != nil && ri.meta.IsClient) || ri.shouldSkip() || ri.opts.skipResponse {
		return resp, nil
	}

	msg, ok := resp.(proto.Message)
	if !ok || msg == nil {
		return resp, nil
	}

	defer func() {
		if recover() == nil {
			return
		}
		ri.LogError(ctx, "fieldbehavior strip panicked", ri.meta.Method(), nil,
			slog.String("side", "response"),
			slog.String("category", sharedrecovery.PanicCategory),
		)
		out = nil
		err = interceptors.NewError(
			status.New(codes.Internal, "Internal Error"),
			sharedrecovery.ErrPanicRecovered,
		)
	}()

	reflected := msg.ProtoReflect()
	if !reflected.IsValid() {
		return resp, nil
	}

	// The strip walk skips irrelevant subtrees and fields it will clear.
	// Clone and marshaling can traverse them, so check the entire graph.
	if !withinResponseDepth(reflected, ri.opts.maxStripDepth) {
		return nil, ri.stripFailure(ctx, pbfieldbehavior.ErrMaxDepthExceeded, "response", "response")
	}
	detectErr := pbfieldbehavior.StripResponse(msg, ri.detectOpts...)
	if detectErr == nil {
		return resp, nil
	}
	if !errors.Is(detectErr, pbfieldbehavior.ErrBehaviorViolation) {
		return nil, ri.stripFailure(ctx, detectErr, "response", "response")
	}

	clone := proto.Clone(msg)
	if err := ri.runStrip(ctx, clone, pbfieldbehavior.StripResponse, "response", "response"); err != nil {
		return nil, err
	}
	return clone, nil
}

func withinResponseDepth(msg protoreflect.Message, remaining int) bool {
	if remaining < 0 {
		return false
	}

	desc := msg.Descriptor()
	fields := desc.Fields()
	for i := range fields.Len() {
		fd := fields.Get(i)
		if isMessageKind(fd.Kind()) && msg.Has(fd) && !fieldWithinResponseDepth(fd, msg.Get(fd), remaining-1) {
			return false
		}
	}
	if desc.ExtensionRanges().Len() == 0 {
		return true
	}

	// Only messages that support extensions need to enumerate populated fields
	// through Range. Extension descriptors are absent from desc.Fields().
	fits := true
	msg.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		if fd.IsExtension() && isMessageKind(fd.Kind()) {
			fits = fieldWithinResponseDepth(fd, v, remaining-1)
		}
		return fits
	})
	return fits
}

func fieldWithinResponseDepth(fd protoreflect.FieldDescriptor, v protoreflect.Value, remaining int) bool {
	switch {
	case fd.IsMap():
		if !isMessageKind(fd.MapValue().Kind()) {
			return true
		}
		fits := true
		v.Map().Range(func(_ protoreflect.MapKey, entry protoreflect.Value) bool {
			fits = withinResponseDepth(entry.Message(), remaining)
			return fits
		})
		return fits
	case fd.IsList():
		list := v.List()
		for i := range list.Len() {
			if !withinResponseDepth(list.Get(i).Message(), remaining) {
				return false
			}
		}
		return true
	default:
		return withinResponseDepth(v.Message(), remaining)
	}
}

func isMessageKind(kind protoreflect.Kind) bool {
	return kind == protoreflect.MessageKind || kind == protoreflect.GroupKind
}

// stripFn is the shared signature of every fieldbehavior.Strip* entry point.
type stripFn = func(proto.Message, ...pbfieldbehavior.Option) error

// runStrip invokes fn against msg with panic recovery and converts errors to
// gRPC status responses. label identifies the strip variant in logs
// ("create" / "update" / "response"); side identifies the wire direction
// ("request" / "response").
func (ri *requestInterceptor) runStrip(ctx context.Context, msg proto.Message, fn stripFn, label, side string) (err error) {
	method := ri.meta.Method()

	defer func() {
		if recover() == nil {
			return
		}
		ri.LogError(ctx, "fieldbehavior strip panicked", method, nil,
			slog.String("side", side),
			slog.String("category", sharedrecovery.PanicCategory),
		)
		err = interceptors.NewError(
			status.New(codes.Internal, "Internal Error"),
			sharedrecovery.ErrPanicRecovered,
		)
	}()

	if stripErr := fn(msg, ri.stripOpts...); stripErr != nil {
		// A request value that breaks its AIP-202 format is the client's
		// error, not the server's.
		if violation, ok := errors.AsType[*pbfieldbehavior.BehaviorViolationError](stripErr); ok && side == "request" {
			ri.LogDebug(ctx, "fieldbehavior request rejected", method,
				slog.String("kind", label),
				slog.Int("violations", len(violation.Violations)),
			)

			return interceptors.NewError(buildViolationStatus(violation), stripErr)
		}

		return ri.stripFailure(ctx, stripErr, label, side)
	}

	ri.LogDebug(ctx, "fieldbehavior strip applied", method,
		slog.String("side", side),
		slog.String("kind", label),
	)

	return nil
}

func (ri *requestInterceptor) stripFailure(ctx context.Context, err error, label, side string) error {
	ri.LogError(ctx, "fieldbehavior strip failed", ri.meta.Method(), nil,
		slog.String("grpc.code", codes.Internal.String()),
		slog.String("classification", diagnostics.ErrorClass(err)),
		slog.String("side", side),
		slog.String("kind", label),
	)
	return interceptors.NewError(status.New(codes.Internal, "Internal Error"), err)
}

// buildViolationStatus renders a request the walk rejects as InvalidArgument
// with an io.altessa.badrequest.v1.BadRequest detail carrying one
// FieldViolation per offending field — the detail the fieldmask and
// protovalidate interceptors emit, so a go-atlas client parses every
// server-side violation the same way.
func buildViolationStatus(err *pbfieldbehavior.BehaviorViolationError) *status.Status {
	violations := make([]*badrequestv1.FieldViolation, 0, len(err.Violations))
	for _, v := range err.Violations {
		violations = append(violations, &badrequestv1.FieldViolation{
			FieldPath: ptr.WrapNonZero(v.Path),
			Message:   ptr.WrapNonZero(v.Reason),
		})
	}

	st := status.New(codes.InvalidArgument, "field behavior violation")
	if detailed, derr := st.WithDetails(&badrequestv1.BadRequest{FieldViolations: violations}); derr == nil {
		return detailed
	}

	return st
}

// RequestContext attaches a snapshot of this message's deferred update plan.
func (ri *requestInterceptor) RequestContext(ctx context.Context) context.Context {
	if plan := ri.plan.Load(); plan != nil {
		return update.ContextWithPlan(ctx, plan)
	}
	return ctx
}
