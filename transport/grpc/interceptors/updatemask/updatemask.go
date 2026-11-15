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
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	pbfieldmask "github.com/altessa-s/go-aip/fieldmask"
)

// Normalizer applies the update_mask of Update requests to their resource
// (see the package documentation). It is safe for concurrent use.
type Normalizer struct {
	extract  extract.UpdateFunc
	policy   []update.Option
	strict   bool
	maskName string
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
	Plan *update.DeferredPlan
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

	t, _ := ctx.Value(trackerKey{}).(*tracker)
	t.enter(n, req)

	raw, resource, writeback, ok := n.extract(ctx, req)
	if !ok || resource == nil {
		if n.strict {
			return Result{}, n.maskError(n.maskName + " is required")
		}

		return Result{}, nil
	}

	raw = t.sent(raw)

	if n.strict {
		switch {
		case raw == nil:
			return Result{}, n.maskError(n.maskName + " is required")
		case len(raw.GetPaths()) == 0:
			return Result{}, n.maskError(n.maskName + " must contain at least one path")
		}
	}

	if writeback == nil {
		return Result{}, interceptors.NewError(status.New(codes.Internal, "Internal Error"),
			errors.New("updatemask: the extractor gives no way to write the effective update_mask back"))
	}

	if previous, ok := t.result(); ok {
		return previous, nil
	}
	plan, err := update.Prepare(raw, resource, n.policy...)
	if err != nil {
		return Result{}, n.convert(err)
	}
	effective := plan.Mask()

	// A writeback may rewrite the mask sent in place: keep a copy of it.
	sent := t.copyOf(raw)
	writeback(effective.ToProtoFieldMask())
	res = Result{Located: true, Resource: resource, Paths: len(effective), Plan: plan}
	t.applied(sent, res)

	return res, nil
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
