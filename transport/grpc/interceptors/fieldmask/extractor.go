// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
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

// newBuiltinExtractors constructs the fallback extractors once at
// interceptor build time. They close over the package-level extract
// options (mask / resource field-name overrides) so descriptor lookups stay
// in sync with the rest of the interceptor.
//
// The read path is a chain of [pbfieldmask.MetadataReadExtractor]
// (AIP-157 modern transport) followed by [pbfieldmask.DefaultReadExtractor]
// (AIP-161 deprecated request-field fallback). When metadataHeader is
// non-empty it overrides the default "x-goog-fieldmask" header on the
// metadata extractor.
//
// The update path stays a single [pbfieldmask.DefaultUpdateExtractor] —
// AIP-134 requires update_mask on the request message and does not sanction
// a side-channel transport.
func newBuiltinExtractors(
	extractOpts []pbfieldmask.ExtractOption,
	metadataHeader string,
) (pbfieldmask.UpdateExtractorFunc, pbfieldmask.ReadExtractorFunc) {
	var metaOpts []pbfieldmask.MetadataExtractorOption
	if metadataHeader != "" {
		metaOpts = append(metaOpts, pbfieldmask.WithMetadataHeader(metadataHeader))
	}

	read := pbfieldmask.ChainReadExtractors(
		pbfieldmask.MetadataReadExtractor(metaOpts...),
		pbfieldmask.DefaultReadExtractor(extractOpts...),
	)

	return pbfieldmask.DefaultUpdateExtractor(extractOpts...), read
}
