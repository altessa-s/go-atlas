// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask

import (
	"context"

	"github.com/altessa-s/go-aip/fieldmask/extract"

	"github.com/altessa-s/go-atlas-core/collections/slices"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/updatemask"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	sharedmetadata "github.com/altessa-s/go-atlas/transport/grpc/interceptors/metadata"
)

// resolveUpdate runs the update extractor of method: its per-method
// override, else the global default, else builtin.
func resolveUpdate(
	ctx context.Context,
	opts *options,
	builtin extract.UpdateFunc,
	method string,
	req proto.Message,
) (*fieldmaskpb.FieldMask, proto.Message, func(*fieldmaskpb.FieldMask), bool) {
	if fn, ok := opts.updateExtractors[method]; ok && fn != nil {
		return fn(ctx, req)
	}

	if opts.defaultUpdateExtractor != nil {
		return opts.defaultUpdateExtractor(ctx, req)
	}

	return builtin(ctx, req)
}

// UpdateNormalizer returns the update_mask normalizer of the interceptor that
// opt configures: it locates the mask with the per-method
// [WithUpdateExtractor] of the call's method (read from the call metadata of
// ctx), else [WithDefaultUpdateExtractor], else the built-in reflection
// extractor honoring [WithMaskFieldName] and [WithResourceFieldName], and
// applies it under [WithUpdateOptions] and [WithStrictMode]. Give it to the fieldbehavior interceptor
// (fieldbehavior.WithUpdateNormalizer) with the options given to
// [ServerInterceptor], so both interceptors apply an update_mask the same way.
func UpdateNormalizer(opt ...Option) *updatemask.Normalizer {
	return newUpdateNormalizer(newOptions(opt...))
}

// newUpdateNormalizer returns the update_mask normalizer of opts.
func newUpdateNormalizer(opts *options) *updatemask.Normalizer {
	builtin, _ := newBuiltinExtractors(extractOptions(opts), opts.metadataReadMaskHeader)

	nopts := []updatemask.Option{
		updatemask.WithExtractor(func(ctx context.Context, req proto.Message) (*fieldmaskpb.FieldMask, proto.Message, func(*fieldmaskpb.FieldMask), bool) {
			return resolveUpdate(ctx, opts, builtin, sharedmetadata.MethodFromContext(ctx), req)
		}),
		updatemask.WithUpdateOptions(opts.updateOptions...),
		updatemask.WithMaskFieldName(updateMaskName(opts)),
	}
	nopts = slices.AppendIf(nopts, opts.strictMode, updatemask.WithStrict())

	return updatemask.New(nopts...)
}

// extractOptions returns the options of the built-in extractors.
func extractOptions(opts *options) []extract.Option {
	var out []extract.Option
	out = slices.AppendIf(out, opts.maskFieldName != "", extract.WithMaskField(opts.maskFieldName))
	out = slices.AppendIf(out, opts.resourceFieldName != "", extract.WithResourceField(opts.resourceFieldName))

	return out
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
// The read path is a chain of [MetadataReadExtractor]
// (AIP-157 modern transport) followed by [extract.DefaultRead]
// (AIP-161 deprecated request-field fallback). When metadataHeader is
// non-empty it overrides the default "x-goog-fieldmask" header on the
// metadata extractor.
//
// The update path stays a single [extract.DefaultUpdate] —
// AIP-134 requires update_mask on the request message and does not sanction
// a side-channel transport.
func newBuiltinExtractors(
	extractOpts []extract.Option,
	metadataHeader string,
) (extract.UpdateFunc, extract.ReadFunc) {
	var metaOpts []MetadataExtractorOption
	metaOpts = slices.AppendIf(metaOpts, metadataHeader != "", WithMetadataHeader(metadataHeader))

	read := extract.ChainRead(
		MetadataReadExtractor(metaOpts...),
		extract.DefaultRead(extractOpts...),
	)

	return updatemask.DefaultExtractor(extractOpts...), read
}
