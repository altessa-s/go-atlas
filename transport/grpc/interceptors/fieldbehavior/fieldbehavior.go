// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldbehavior

import (
	"context"
	"log/slog"

	"github.com/altessa-s/go-atlas/core/runtime/panics"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/driver"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	pbfieldbehavior "github.com/altessa-s/go-atlas/domain/proto/fieldbehavior"
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
	_ driver.DrivenInterceptor = (*interceptor)(nil)
	_ driver.DriverStream      = (*requestInterceptor)(nil)
	_ interceptors.Interceptor = (*interceptor)(nil)
)

// interceptor implements the fieldbehavior gRPC interceptor.
type interceptor struct {
	interceptors.BaseInterceptor
	opts      *options
	stripOpts []pbfieldbehavior.Option
}

// Dependencies returns interceptors that fieldbehavior requires to run before
// it. Metadata is needed for the method name, and auth must reject
// unauthenticated requests before any payload mutation runs.
func (i *interceptor) Dependencies() []string {
	return []string{sharedmetadata.Name(), auth.Name()}
}

// requestInterceptor holds per-call state captured at PreCall time.
type requestInterceptor struct {
	*interceptor
	meta *sharedmetadata.CallMetadata
	kind Kind
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

// ServerInterceptor returns a server interceptor that strips field_behavior
// annotations from gRPC requests and responses based on the method name.
func ServerInterceptor(opt ...Option) interceptors.ServerInterceptor {
	opts := newOptions(opt...)

	stripOpts := []pbfieldbehavior.Option{
		pbfieldbehavior.WithMaxDepth(opts.maxStripDepth),
	}

	ic := &interceptor{
		BaseInterceptor: interceptors.NewBaseInterceptorWithFilter(
			interceptorName,
			opts.ignoreMethods,
			opts.ignorePatterns,
			opts.logger,
		),
		opts:      opts,
		stripOpts: stripOpts,
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

// PostCall runs the response-side Strip after the handler returns. A handler
// error short-circuits the strip — the response body, if any, is not exposed
// to the wire by gRPC in that case.
func (ri *requestInterceptor) PostCall(ctx context.Context, resp any, err error) error {
	if err != nil {
		return err
	}

	if ri.meta != nil && ri.meta.IsClient {
		return nil
	}

	if ri.shouldSkip() || ri.opts.skipResponse {
		return nil
	}

	if stripErr := ri.stripResponse(ctx, resp); stripErr != nil {
		return stripErr
	}

	return nil
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

// PostMsgSent runs on every outgoing streaming message and applies the
// response-side Strip mirroring the unary PostCall path.
func (ri *requestInterceptor) PostMsgSent(ctx context.Context, resp any, err error) error {
	if err != nil {
		return err
	}

	if ri.meta != nil && ri.meta.IsClient {
		return nil
	}

	if ri.shouldSkip() || ri.opts.skipResponse {
		return nil
	}

	return ri.stripResponse(ctx, resp)
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
	var fn stripFn

	switch ri.kind {
	case KindCreate:
		fn = pbfieldbehavior.StripCreate
	case KindUpdate:
		fn = pbfieldbehavior.StripUpdate
	case KindNone, KindSkip:
		return nil
	}

	msg, ok := req.(proto.Message)
	if !ok {
		return nil
	}

	return ri.runStrip(ctx, msg, fn, ri.kind.String(), "request")
}

// stripResponse applies StripResponse to resp.
func (ri *requestInterceptor) stripResponse(ctx context.Context, resp any) error {
	msg, ok := resp.(proto.Message)
	if !ok {
		return nil
	}

	return ri.runStrip(ctx, msg, pbfieldbehavior.StripResponse, "response", "response")
}

// stripFn is the shared signature of every fieldbehavior.Strip* entry point.
type stripFn = func(proto.Message, ...pbfieldbehavior.Option) error

// runStrip invokes fn against msg with panic recovery and converts errors to
// gRPC status responses. label identifies the strip variant in logs
// ("create" / "update" / "response"); side identifies the wire direction
// ("request" / "response").
func (ri *requestInterceptor) runStrip(ctx context.Context, msg proto.Message, fn stripFn, label, side string) (err error) {
	method := ri.meta.Method()

	defer panics.HandleWithOpts(ctx, panics.NewHandleOpts().SetReallyPanic(false), func(_ context.Context, r any) {
		ri.LogError(ctx, "fieldbehavior strip panicked", method, nil,
			slog.String("side", side),
			slog.Any("panic", r),
		)
		err = interceptors.NewError(
			status.New(codes.Internal, "Internal Error"),
			nil,
		)
	})

	if stripErr := fn(msg, ri.stripOpts...); stripErr != nil {
		ri.LogError(ctx, "fieldbehavior strip failed", method, stripErr,
			slog.String("side", side),
			slog.String("kind", label),
		)

		return interceptors.NewError(
			status.New(codes.Internal, "Internal Error"),
			stripErr,
		)
	}

	ri.LogDebug(ctx, "fieldbehavior strip applied", method,
		slog.String("side", side),
		slog.String("kind", label),
	)

	return nil
}
