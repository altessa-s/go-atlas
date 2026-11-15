// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldbehavior

import (
	"context"
	"errors"
	"log/slog"
	"slices"

	"github.com/altessa-s/go-aip/fieldinfo"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	pbfieldbehavior "github.com/altessa-s/go-aip/fieldbehavior"
)

// stripUpdateRequest prepares the resource and retains its deferred plan.
// Outside the resource, and on lookup misses, StripUpdate clears OUTPUT_ONLY
// while preserving protected values for handler checks. Formats are checked
// on retained values after Prepare sanitizes the resource, including for
// explicit empty and dropped-only masks. Do not strip the prepared resource
// again: its preservation policy and captured input belong to the plan.
func (ri *requestInterceptor) stripUpdateRequest(ctx context.Context, msg proto.Message) error {
	method := ri.meta.Method()

	res, err := ri.updates.Normalize(ctx, msg)
	ri.plan.Store(res.Plan)
	if err != nil {
		if status.Code(err) == codes.Internal {
			ri.LogError(ctx, "fieldbehavior update_mask apply failed", method, err)
		} else {
			ri.LogDebug(ctx, "fieldbehavior update request rejected", method, slog.String("error", err.Error()))
		}

		return err
	}

	if !res.Located {
		return ri.runStrip(ctx, msg, pbfieldbehavior.StripUpdate, ri.kind.String(), "request")
	}

	outside := func(m proto.Message, opts ...pbfieldbehavior.Option) error {
		return stripOutside(m, res.Resource, opts...)
	}
	if err := ri.runStrip(ctx, msg, outside, ri.kind.String(), "request"); err != nil {
		return err
	}

	return ri.checkUpdateFormats(ctx, msg)
}

// checkUpdateFormats validates AIP-202 formats of all retained request values.
func (ri *requestInterceptor) checkUpdateFormats(ctx context.Context, req proto.Message) error {
	err := fieldinfo.Validate(req, fieldinfo.WithMaxDepth(ri.opts.maxStripDepth))
	if err == nil {
		return nil
	}

	invalid, ok := errors.AsType[*fieldinfo.ViolationError](err)
	if !ok {
		ri.LogError(ctx, "fieldbehavior format check failed", ri.meta.Method(), err)
		return interceptors.NewError(status.New(codes.Internal, "Internal Error"), err)
	}

	violations := make([]pbfieldbehavior.BehaviorViolation, 0, len(invalid.Violations))
	for _, v := range invalid.Violations {
		violations = append(violations, pbfieldbehavior.BehaviorViolation{Path: v.Path, Format: v.Format, Reason: "value " + v.Reason + " (" + v.Format + ")"})
	}
	ri.LogDebug(ctx, "fieldbehavior update value breaks its format", ri.meta.Method(), slog.Int("violations", len(violations)))

	return interceptors.NewError(buildViolationStatus(&pbfieldbehavior.BehaviorViolationError{Violations: violations}), err)
}

// errResourceOutsideRequest reports an update extractor whose resource is
// not a message the request holds: the mask was applied to something the
// handler never sees.
var errResourceOutsideRequest = errors.New("fieldbehavior: the update resource is not held by the request")

// stripOutside runs go-aip fieldbehavior.StripUpdate on the fields of req
// outside resource, a message req holds through singular message fields:
// resource is detached from req for the walk and put back afterwards. A
// resource that is req itself leaves nothing outside. Formats are checked
// afterwards, on the whole request.
func stripOutside(req, resource proto.Message, opts ...pbfieldbehavior.Option) error {
	if req == resource {
		return nil
	}

	route, ok := pathTo(req.ProtoReflect(), resource.ProtoReflect(), maxResourceDepth)
	if !ok {
		return errResourceOutsideRequest
	}

	parent := req.ProtoReflect()
	for _, fd := range route[:len(route)-1] {
		parent = parent.Get(fd).Message()
	}
	last := route[len(route)-1]
	held := parent.Get(last)
	parent.Clear(last)
	defer func() {
		// StripUpdate can replace the request tree, detaching the old parent.
		parent := req.ProtoReflect()
		for _, fd := range route[:len(route)-1] {
			parent = parent.Mutable(fd).Message()
		}

		parent.Set(last, held)
	}()

	return pbfieldbehavior.StripUpdate(req, append(slices.Clone(opts), pbfieldbehavior.WithFieldFormats(false))...)
}

// maxResourceDepth bounds the search for the resource in a request.
const maxResourceDepth = 8

// pathTo returns the singular message fields leading from m to target, which
// is found by identity, and whether it was found within depth levels.
func pathTo(m, target protoreflect.Message, depth int) ([]protoreflect.FieldDescriptor, bool) {
	if depth == 0 {
		return nil, false
	}

	var (
		route []protoreflect.FieldDescriptor
		found bool
	)
	m.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		if fd.Message() == nil || fd.IsList() || fd.IsMap() {
			return true
		}
		child := v.Message()
		if child.Interface() == target.Interface() {
			route, found = []protoreflect.FieldDescriptor{fd}, true
			return false
		}
		if sub, ok := pathTo(child, target, depth-1); ok {
			route, found = append([]protoreflect.FieldDescriptor{fd}, sub...), true
			return false
		}
		return true
	})

	return route, found
}
