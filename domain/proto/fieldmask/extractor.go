// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask

import (
	"context"
	"slices"

	"github.com/altessa-s/go-atlas/core/types/nilcheck"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// UpdateExtractorFunc locates the update_mask, the resource sub-message, and
// (optionally) a writeback closure on req. The writeback closure is invoked
// by the caller after [FieldMask.ApplyUpdateMask] cleans OUTPUT_ONLY entries;
// return nil from the writeback slot to skip writeback. Return ok=false to
// fall through to a silent passthrough.
//
// The function operates on [proto.Message] and has no gRPC dependency — it
// is the transport-agnostic shape consumed by
// transport/grpc/interceptors/fieldmask but reusable from any caller (HTTP
// handlers, CLI tools, batch jobs).
type UpdateExtractorFunc func(ctx context.Context, req proto.Message) (
	mask *fieldmaskpb.FieldMask,
	resource proto.Message,
	writeback func(*fieldmaskpb.FieldMask),
	ok bool,
)

// ReadExtractorFunc locates the read_mask on req. Return ok=false to fall
// through to a silent passthrough.
type ReadExtractorFunc func(ctx context.Context, req proto.Message) (
	mask *fieldmaskpb.FieldMask,
	ok bool,
)

// NewUpdateExtractor builds an [UpdateExtractorFunc] from typed getters. The
// returned function:
//
//   - type-asserts the request to ReqT; returns ok=false on type mismatch;
//   - calls getMask(req); returns ok=false when the mask is nil;
//   - calls getResource(req); returns ok=false when the resource is the
//     typed-nil interface;
//   - if setMask is non-nil, wraps it in a writeback closure that bakes in
//     the request pointer so the caller can write the cleaned mask back
//     without re-type-asserting.
//
// Use this builder for requests that do not match the canonical AIP-134
// shape — for example when the mask lives inside a sibling Options
// sub-message:
//
//	fieldmask.NewUpdateExtractor(
//	    func(r *pb.UpdateBucketRequest) *fieldmaskpb.FieldMask { return r.GetOptions().GetUpdateMask() },
//	    func(r *pb.UpdateBucketRequest) *pb.Bucket             { return r.GetBucket() },
//	    func(r *pb.UpdateBucketRequest, m *fieldmaskpb.FieldMask) { r.Options.UpdateMask = m },
//	)
func NewUpdateExtractor[ReqT, ResT proto.Message](
	getMask func(ReqT) *fieldmaskpb.FieldMask,
	getResource func(ReqT) ResT,
	setMask func(ReqT, *fieldmaskpb.FieldMask),
) UpdateExtractorFunc {
	return func(_ context.Context, raw proto.Message) (*fieldmaskpb.FieldMask, proto.Message, func(*fieldmaskpb.FieldMask), bool) {
		req, ok := raw.(ReqT)
		if !ok {
			return nil, nil, nil, false
		}

		if getMask == nil || getResource == nil {
			return nil, nil, nil, false
		}

		mask := getMask(req)
		if mask == nil {
			return nil, nil, nil, false
		}

		res := getResource(req)
		if nilcheck.IsNil(res) {
			return nil, nil, nil, false
		}

		var writeback func(*fieldmaskpb.FieldMask)
		if setMask != nil {
			writeback = func(m *fieldmaskpb.FieldMask) {
				setMask(req, m)
			}
		}

		return mask, res, writeback, true
	}
}

// NewReadExtractor builds a [ReadExtractorFunc] from a typed getter. Returns
// ok=false on request-type mismatch or when getMask returns nil.
//
//	fieldmask.NewReadExtractor(
//	    func(r *pb.GetBucketRequest) *fieldmaskpb.FieldMask { return r.GetOptions().GetReadMask() },
//	)
func NewReadExtractor[ReqT proto.Message](
	getMask func(ReqT) *fieldmaskpb.FieldMask,
) ReadExtractorFunc {
	return func(_ context.Context, raw proto.Message) (*fieldmaskpb.FieldMask, bool) {
		req, ok := raw.(ReqT)
		if !ok {
			return nil, false
		}

		if getMask == nil {
			return nil, false
		}

		mask := getMask(req)
		if mask == nil {
			return nil, false
		}

		return mask, true
	}
}

// ChainReadExtractors returns a [ReadExtractorFunc] that invokes each fn in
// order and returns the first result that signals ok=true. ok=false from
// every fn yields ok=false, which the gRPC interceptor treats as a silent
// passthrough.
//
// Nil entries are skipped. ChainReadExtractors called with no functions, or
// with only nil entries, returns a no-op extractor that always reports
// ok=false — callers can rely on the result being non-nil and safe to call.
//
// Use the chain to combine modern transports (gRPC metadata per AIP-157)
// with legacy fallbacks (deprecated request-message read_mask per AIP-161)
// without forcing the caller to pick one source.
func ChainReadExtractors(fns ...ReadExtractorFunc) ReadExtractorFunc {
	compact := slices.DeleteFunc(slices.Clone(fns), func(f ReadExtractorFunc) bool { return f == nil })
	if len(compact) == 0 {
		return func(context.Context, proto.Message) (*fieldmaskpb.FieldMask, bool) {
			return nil, false
		}
	}

	return func(ctx context.Context, req proto.Message) (*fieldmaskpb.FieldMask, bool) {
		for _, fn := range compact {
			if mask, ok := fn(ctx, req); ok {
				return mask, true
			}
		}
		return nil, false
	}
}

// ChainUpdateExtractors mirrors [ChainReadExtractors] for
// [UpdateExtractorFunc]. Exported for symmetry and per-service composition;
// the gRPC interceptor's built-in update path is a single extractor because
// AIP-134 requires update_mask on the request message and does not sanction
// a side-channel transport.
//
// As with [ChainReadExtractors], a call with no functions or only nil
// entries returns a no-op extractor that always reports ok=false.
func ChainUpdateExtractors(fns ...UpdateExtractorFunc) UpdateExtractorFunc {
	compact := slices.DeleteFunc(slices.Clone(fns), func(f UpdateExtractorFunc) bool { return f == nil })
	if len(compact) == 0 {
		return func(context.Context, proto.Message) (*fieldmaskpb.FieldMask, proto.Message, func(*fieldmaskpb.FieldMask), bool) {
			return nil, nil, nil, false
		}
	}

	return func(ctx context.Context, req proto.Message) (*fieldmaskpb.FieldMask, proto.Message, func(*fieldmaskpb.FieldMask), bool) {
		for _, fn := range compact {
			if mask, resource, writeback, ok := fn(ctx, req); ok {
				return mask, resource, writeback, true
			}
		}
		return nil, nil, nil, false
	}
}

// DefaultUpdateExtractor wraps [ExtractUpdateMask] and [SetUpdateMask] in
// the [UpdateExtractorFunc] shape. Used by the gRPC interceptor as the
// bottom-of-stack fallback when no per-method override or global default is
// configured. opts are forwarded to both helpers, so [WithMaskField] /
// [WithResourceField] continue to work via the default extractor.
func DefaultUpdateExtractor(opts ...ExtractOption) UpdateExtractorFunc {
	return func(_ context.Context, req proto.Message) (*fieldmaskpb.FieldMask, proto.Message, func(*fieldmaskpb.FieldMask), bool) {
		mask, resource, ok := ExtractUpdateMask(req, opts...)
		if !ok {
			return nil, nil, nil, false
		}

		// Invariant: ExtractUpdateMask just resolved the mask field on req with
		// the same opts, so SetUpdateMask cannot return ErrFieldNotSettable
		// here. The discard is intentional and safe by construction.
		writeback := func(m *fieldmaskpb.FieldMask) {
			_ = SetUpdateMask(req, m, opts...)
		}

		return mask, resource, writeback, true
	}
}

// DefaultReadExtractor wraps [ExtractReadMask] in the [ReadExtractorFunc]
// shape. Used by the gRPC interceptor as the bottom-of-stack fallback.
func DefaultReadExtractor(opts ...ExtractOption) ReadExtractorFunc {
	return func(_ context.Context, req proto.Message) (*fieldmaskpb.FieldMask, bool) {
		return ExtractReadMask(req, opts...)
	}
}
