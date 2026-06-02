// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask

import (
	"context"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	pbfieldmask "github.com/altessa-s/go-atlas/domain/proto/fieldmask"
)

// resolveUpdate returns the effective update extractor for the current call.
// Resolution order: per-method override → global default → built-in
// reflection extractor.
func (ri *requestInterceptor) resolveUpdate(
	ctx context.Context,
	req proto.Message,
) (*fieldmaskpb.FieldMask, proto.Message, func(*fieldmaskpb.FieldMask), bool) {
	if fn, ok := ri.opts.updateExtractors[ri.meta.FullyMethodName]; ok && fn != nil {
		return fn(ctx, req)
	}

	if ri.opts.defaultUpdateExtractor != nil {
		return ri.opts.defaultUpdateExtractor(ctx, req)
	}

	return ri.builtinUpdateExtractor(ctx, req)
}

// resolveRead returns the effective read extractor for the current call.
// Resolution order: per-method override → global default → built-in
// reflection extractor.
func (ri *requestInterceptor) resolveRead(ctx context.Context, req proto.Message) (*fieldmaskpb.FieldMask, bool) {
	if fn, ok := ri.opts.readExtractors[ri.meta.FullyMethodName]; ok && fn != nil {
		return fn(ctx, req)
	}

	if ri.opts.defaultReadExtractor != nil {
		return ri.opts.defaultReadExtractor(ctx, req)
	}

	return ri.builtinReadExtractor(ctx, req)
}

// newBuiltinExtractors constructs the reflection-based fallback extractors
// once at interceptor build time. They close over the package-level extract
// options (mask / resource field-name overrides) so descriptor lookups stay
// in sync with the rest of the interceptor.
func newBuiltinExtractors(extractOpts []pbfieldmask.ExtractOption) (pbfieldmask.UpdateExtractorFunc, pbfieldmask.ReadExtractorFunc) {
	return pbfieldmask.DefaultUpdateExtractor(extractOpts...),
		pbfieldmask.DefaultReadExtractor(extractOpts...)
}
