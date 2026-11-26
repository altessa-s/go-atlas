// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package updatemask

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/altessa-s/go-aip/fieldmask/extract"
	"github.com/altessa-s/go-aip/fieldmask/update"
	"github.com/altessa-s/proto-gen-go/io/altessa/badrequest/v1"

	"github.com/altessa-s/go-atlas-core/types/ptr"
	"github.com/altessa-s/go-atlas/internal/transport/updatevalidation"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	pbfieldmask "github.com/altessa-s/go-aip/fieldmask"
	sharedmetadata "github.com/altessa-s/go-atlas/transport/grpc/interceptors/metadata"
)

// Normalizer applies the update_mask of Update requests to their resource
// (see the package documentation). It is safe for concurrent use.
type Normalizer struct {
	extract  extract.UpdateFunc
	policy   []update.Option
	strict   bool
	maskName string
	spec     updatevalidation.SpecLookup
}

// New returns a Normalizer configured by opt.
func New(opt ...Option) *Normalizer {
	o := newOptions(opt...)
	if o.extractor == nil {
		o.extractor = DefaultExtractor()
	}

	return &Normalizer{
		extract: o.extractor,
		policy: slices.Concat(o.updateOptions, []update.Option{
			update.WithUpdateMaskFieldName(o.maskFieldName),
			update.WithStrict(o.strict),
		}),
		strict:   o.strict,
		maskName: o.maskFieldName,
		spec:     o.specLookup,
	}
}

// Result describes a request a [Normalizer] handled.
type Result struct {
	// Located reports whether the extractor found the update_mask (or its
	// omission) and the resource of the request. A request it did not locate
	// is left untouched.
	Located bool
	// Resource is the resource the mask was applied to, a message held by
	// the request; nil when the request was not located.
	Resource proto.Message
	// Paths counts initial effective paths. Zero does not mean that deferred
	// comparison or replacement has finished.
	Paths int
	// Plan retains original values and clear intent for completion by the handler.
	Plan      *update.DeferredPlan
	selection *updatevalidation.Selection
	legacy    *legacyPlan
}

// Normalize applies the update_mask of req to its resource with
// update.Prepare and writes the effective mask back to req. On an error req
// is left untouched and the error is a gRPC status error
// ([interceptors.NewError]): InvalidArgument for a mask or a resource the
// client got wrong, Internal for a failure of the server (a panic of the
// extractor included).
//
// A request whose mask or resource the extractor does not locate is left
// untouched with Located false, or refused in strict mode. On a context
// [Track] marks, a received message another Normalizer already applied is
// reused with its original plan; every Normalizer of a call
// handles a received message once, so a Normalizer given a request again
// takes it for the next message of a stream, received into the same request.
func (n *Normalizer) Normalize(ctx context.Context, req proto.Message) (res Result, err error) {
	defer func() {
		if r := recover(); r != nil {
			res, err = Result{}, interceptors.NewError(status.New(codes.Internal, "Internal Error"), fmt.Errorf("updatemask: panic: %v", r))
		}
	}()

	if spec := n.registeredSpec(ctx); spec != nil {
		return n.normalizeRegistered(ctx, req, spec)
	}

	t, _ := ctx.Value(trackerKey{}).(*tracker)
	t.enter(n, req)

	raw, resource, writeback, ok := n.extract(ctx, req)
	if !ok || resource == nil {
		if n.strict {
			return Result{}, n.maskError(n.maskName + " is required")
		}

		return Result{}, nil
	}

	previous := legacyFromContext(ctx, req)
	if previous != nil {
		raw = previous.sent
	} else {
		raw = t.sent(raw)
	}
	if presenceErr := n.checkMaskPresence(raw); presenceErr != nil {
		return Result{}, presenceErr
	}

	if writeback == nil {
		return Result{}, interceptors.NewError(status.New(codes.Internal, "Internal Error"),
			errors.New("updatemask: the extractor gives no way to write the effective update_mask back"))
	}

	if previous != nil {
		return Result{Located: true, Resource: previous.resource, Paths: previous.paths, Plan: previous.plan, legacy: previous}, nil
	}
	if tracked, ok := t.result(); ok {
		return tracked, nil
	}
	plan, err := update.Prepare(raw, resource, n.policy...)
	if err != nil {
		return Result{}, n.convert(err)
	}
	effective := plan.Mask()

	// A writeback may rewrite the mask sent in place: keep a copy of it.
	res = Result{Located: true, Resource: resource, Paths: len(effective), Plan: plan}
	if t == nil {
		if meta, _ := sharedmetadata.FromContext(ctx); meta != nil && !meta.IsStream && !meta.IsClient {
			res.legacy = &legacyPlan{request: req, resource: resource, plan: plan, paths: res.Paths, sent: proto.CloneOf(raw)}
		}
	}
	sent := t.copyOf(raw)
	writeback(effective.ToProtoFieldMask())
	t.applied(sent, res)

	return res, nil
}

// registeredSpec checks the RPC registration before taking the selection path.
func (n *Normalizer) registeredSpec(ctx context.Context) *update.Spec {
	if n.spec == nil {
		return nil
	}
	meta, _ := sharedmetadata.FromContext(ctx)
	if meta == nil || meta.IsStream || meta.IsClient {
		return nil
	}
	return n.spec(ctx)
}

// Wire the private interceptor operations without adding them to the public
// Normalizer, Result, or options API.
func init() {
	updatevalidation.NewSpecLookupOption = func(lookup updatevalidation.SpecLookup) any {
		return Option(func(o *options) { o.specLookup = lookup })
	}
	updatevalidation.HasRegisteredSpec = func(ctx context.Context, normalizer any) bool {
		return normalizer.(*Normalizer).registeredSpec(ctx) != nil //nolint:errcheck // Interceptors supply their normalizer.
	}
	updatevalidation.Normalize = func(ctx context.Context, normalizer any, request proto.Message, stream bool,
	) (context.Context, updatevalidation.Normalization, error) {
		n := normalizer.(*Normalizer) //nolint:errcheck // Interceptors supply their normalizer.
		res, err := n.Normalize(ctx, request)
		if !stream && err == nil {
			ctx = res.requestContext(ctx)
		}
		return ctx, updatevalidation.Normalization{Located: res.Located, Resource: res.Resource, Paths: res.Paths, Plan: res.Plan}, err
	}
}

func (r Result) requestContext(ctx context.Context) context.Context {
	if r.selection != nil {
		return updatevalidation.WithSelection(ctx, r.selection)
	}
	if r.legacy == nil {
		return update.ContextWithPlan(ctx, r.Plan)
	}
	if legacyFromContext(ctx, r.legacy.request) == r.legacy {
		return ctx
	}
	return context.WithValue(update.ContextWithPlan(ctx, r.Plan), legacyPlanKey{}, r.legacy)
}

type legacyPlanKey struct{}

// legacyPlan is the immutable unary handoff. The second normalizer still runs
// its extractor and strict check using the original client mask.
type legacyPlan struct {
	request, resource proto.Message
	plan              *update.DeferredPlan
	sent              *fieldmaskpb.FieldMask
	paths             int
}

func legacyFromContext(ctx context.Context, req proto.Message) *legacyPlan {
	plan, _ := ctx.Value(legacyPlanKey{}).(*legacyPlan)
	if plan != nil && plan.request == req {
		return plan
	}
	return nil
}

func (n *Normalizer) normalizeRegistered(ctx context.Context, req proto.Message, spec *update.Spec) (Result, error) {
	if sel := updatevalidation.FromContext(ctx, req); sel != nil && sel.Spec == spec {
		if err := n.checkMaskPresence(sel.OriginalMask); err != nil {
			return Result{}, err
		}
		return Result{Located: true, Resource: sel.Resource, selection: sel}, nil
	}

	raw, resource, _, ok := n.extract(ctx, req)
	if !ok || resource == nil || !resource.ProtoReflect().IsValid() {
		if n.strict {
			return Result{}, n.maskError(n.maskName + " is required")
		}
		return Result{}, nil
	}
	if err := n.checkMaskPresence(raw); err != nil {
		return Result{}, err
	}
	route, ok := updatevalidation.ResourcePath(req, resource)
	if !ok {
		return Result{}, n.convert(errors.New("updatemask: the update resource is not held by the request"))
	}
	sel := &updatevalidation.Selection{Request: req, Resource: resource, Route: route, Spec: spec, OriginalMask: proto.CloneOf(raw)}
	sel.Mask, sel.Err = spec.ResolveMask(resource, raw)
	if sel.Err != nil {
		if _, ok := errors.AsType[*update.MaskPathError](sel.Err); !ok {
			return Result{}, n.convert(sel.Err)
		}
		sel.MaskError = n.maskError(sel.Err.Error())
		if !updatevalidation.DefersMaskError(ctx) {
			return Result{}, sel.MaskError
		}
	}

	if err := sel.StripOutputOnly(); err != nil {
		return Result{}, n.convert(err)
	}
	return Result{Located: true, Resource: resource, selection: sel}, nil
}

func (n *Normalizer) checkMaskPresence(raw *fieldmaskpb.FieldMask) error {
	if !n.strict {
		return nil
	}
	switch {
	case raw == nil:
		return n.maskError(n.maskName + " is required")
	case len(raw.GetPaths()) == 0:
		return n.maskError(n.maskName + " must contain at least one path")
	default:
		return nil
	}
}

// maskError refuses the update_mask as a whole, naming the mask field.
func (n *Normalizer) maskError(message string) error {
	return interceptors.NewError(n.maskStatus([]*errdetails.BadRequest_FieldViolation{{
		Field:       n.maskName,
		Description: message,
	}}, message), nil)
}

// convert maps an error of update.Prepare to a gRPC status error. A strict
// refusal and a rejected path are InvalidArgument with a google.rpc.BadRequest
// naming the mask field; a field behavior violation is InvalidArgument with an
// io.altessa.badrequest.v1.BadRequest naming each field; anything else is
// Internal.
func (n *Normalizer) convert(err error) error {
	if strict, ok := errors.AsType[*update.StrictError](err); ok {
		violations := make([]*errdetails.BadRequest_FieldViolation, 0, len(strict.Violations))
		for _, v := range strict.Violations {
			violations = append(violations, &errdetails.BadRequest_FieldViolation{
				Field:       n.maskName,
				Description: describe(v.Path, v.Reason),
			})
		}

		return interceptors.NewError(n.maskStatus(violations, "invalid "+n.maskName), err)
	}

	if behavior, ok := errors.AsType[*update.BehaviorError](err); ok {
		violations := make([]*badrequestv1.FieldViolation, 0, len(behavior.Violations))
		for _, v := range behavior.Violations {
			violations = append(violations, &badrequestv1.FieldViolation{
				FieldPath: ptr.WrapNonZero(v.Path),
				Message:   ptr.WrapNonZero(v.Reason),
			})
		}

		st := status.New(codes.InvalidArgument, "field behavior violation")
		if detailed, derr := st.WithDetails(&badrequestv1.BadRequest{FieldViolations: violations}); derr == nil {
			st = detailed
		}

		return interceptors.NewError(st, err)
	}

	if _, ok := errors.AsType[*pbfieldmask.ValidationError](err); ok {
		return interceptors.NewError(n.maskStatus([]*errdetails.BadRequest_FieldViolation{{
			Field:       n.maskName,
			Description: err.Error(),
		}}, err.Error()), err)
	}

	return interceptors.NewError(status.New(codes.Internal, "Internal Error"), err)
}

// maskStatus is InvalidArgument with message and a google.rpc.BadRequest
// of violations.
func (n *Normalizer) maskStatus(violations []*errdetails.BadRequest_FieldViolation, message string) *status.Status {
	st := status.New(codes.InvalidArgument, message)
	if detailed, err := st.WithDetails(&errdetails.BadRequest{FieldViolations: violations}); err == nil {
		return detailed
	}

	return st
}

// describe joins the path a violation names, if any, to its reason.
func describe(path, reason string) string {
	if path == "" {
		return reason
	}

	return path + ": " + reason
}

// trackerKey is the context key of the request tracker.
type trackerKey struct{}

// tracker follows the received message the normalizers of a call handle: the
// request holding it, the normalizers that handled it and, once one of them
// applied it, the update_mask its client sent.
//
// A streaming handler may receive every message into one request, so the
// request alone does not tell one message from the next. Every normalizer of
// a call handles a received message once: a normalizer handed the request it
// already handled is handed the next message.
type tracker struct {
	mu   sync.Mutex
	req  proto.Message
	seen []*Normalizer
	// done reports that a normalizer applied the message; raw is then the
	// mask sent with it, copied, nil when omitted.
	done bool
	raw  *fieldmaskpb.FieldMask
	res  Result
}

// Track returns ctx with a request tracker, or ctx itself when it holds one
// already. Every interceptor that normalizes Update requests calls it when a
// call starts, so that interceptors of one chain share the tracker. Each of
// them normalizes with a [Normalizer] of its own.
func Track(ctx context.Context) context.Context {
	if _, ok := ctx.Value(trackerKey{}).(*tracker); ok {
		return ctx
	}

	return context.WithValue(ctx, trackerKey{}, &tracker{})
}

// enter records that n handles the message req holds. It is a new message
// when req is not the request tracked or n already handled the one tracked.
// A nil t tracks nothing.
func (t *tracker) enter(n *Normalizer, req proto.Message) {
	if t == nil {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.req != req || slices.Contains(t.seen, n) {
		t.req, t.seen, t.done, t.raw = req, t.seen[:0], false, nil
		t.res = Result{}
	}

	t.seen = append(t.seen, n)
}

// sent returns the update_mask the client sent with the message tracked: the
// mask recorded when a normalizer applied it, and raw, the mask the request
// holds, when none did yet.
func (t *tracker) sent(raw *fieldmaskpb.FieldMask) *fieldmaskpb.FieldMask {
	if t == nil {
		return raw
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.done {
		return t.raw
	}

	return raw
}

// copyOf returns the mask to record for the message tracked once it is
// applied from sent: the mask recorded already, or a copy of sent.
func (t *tracker) copyOf(sent *fieldmaskpb.FieldMask) *fieldmaskpb.FieldMask {
	if t == nil {
		return nil
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.done || sent == nil {
		return t.raw
	}

	return proto.CloneOf(sent)
}

// applied records that a normalizer applied the message tracked from sent.
func (t *tracker) applied(sent *fieldmaskpb.FieldMask, res Result) {
	if t == nil {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	t.done, t.raw, t.res = true, sent, res
}

// result returns the first successful normalization of this message.
func (t *tracker) result() (Result, bool) {
	if t == nil {
		return Result{}, false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.res, t.done
}
