// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask

//go:generate go tool optgen generate --type=options

import (
	"log/slog"
	"regexp"

	"github.com/altessa-s/go-aip/fieldmask/extract"
	"github.com/altessa-s/go-aip/fieldmask/update"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/defaults"
)

// Use defaults package for optgen code generation.
var _ = defaults.IgnorePatterns

// options holds configuration for the fieldmask interceptor.
type options struct {
	// updateSpecs binds unary Update methods to handler-owned specifications.
	updateSpecs map[string]*update.Spec `optgen:"manual"`

	// updateOptions configures the update check: the mask policy (field
	// behavior resolver, identity, immutable and implied-skip fields, atomic
	// messages), path validation and limits. One policy serves strict
	// validation, implied masks and deferred planning; the
	// interceptor's update_mask field name is applied last.
	updateOptions []update.Option `optgen:"append" optval:"clone"`
	// WithMethodKind overrides the default classification for fullMethod. The key
	// is the value gRPC passes as info.FullMethod, including the leading slash
	// ("/x.v1.X/UpdateBucket"). Calling the option multiple times accumulates
	// overrides; later calls replace earlier ones for the same method.
	methodKinds map[string]Kind `opt:"MethodKind" optgen:"put"`
	// WithResourceReadMask makes read masks address the resource (AIP-157) instead of the response message.
	// On List*, Search* and BatchGet* methods whose response has a single repeated message field, the paths
	// address each element of that field and the other response fields (next_page_token, total_size,
	// unreachable) are kept; on other read methods they address the response, as by default. Before the
	// handler runs, the paths are checked against the resource resolved from the method's output type in the
	// global registry; an invalid mask is the InvalidArgument INVALID_FIELD_MASK refusal with an added
	// io.altessa.badrequest.v1.BadRequest violation on the read mask field. An omitted mask or "*" alone
	// selects every field.
	resourceReadMask bool
	// WithSkipReadMask disables the response-side Filter pass that runs on
	// KindRead methods. Use it when the handler already applies the read mask
	// itself and the duplicate walk is wasted work.
	skipReadMask bool `optgen:"default=false"`

	// WithMaskFieldName selects an exact declared mask field name without fallback.
	// Empty uses a singular google.protobuf.FieldMask declared as update_mask /
	// read_mask, falling back to updateMask / readMask only when the corresponding
	// snake_case name is not declared.
	maskFieldName string

	// resourceFieldName overrides the conventional resource carrier field
	// name on Update requests. Empty means require exactly one non-mask
	// message field declared on the request.
	resourceFieldName string

	// updateExtractors holds per-method overrides for locating the
	// update_mask + resource. Populated by [WithUpdateExtractor]. When the
	// current method has an entry it wins over [defaultUpdateExtractor] and
	// the built-in reflection extractor.
	// WithUpdateExtractor inserts or replaces one method entry after validating inputs.
	updateExtractors map[string]extract.UpdateFunc `optgen:"manual,default=nil"`

	// readExtractors holds per-method overrides for locating the
	// read_mask. Populated by [WithReadExtractor].
	// WithReadExtractor inserts or replaces one method entry after validating inputs.
	readExtractors map[string]extract.ReadFunc `optgen:"manual,default=nil"`
	// WithDefaultUpdateExtractor replaces the built-in reflection-based update
	// extractor with fn. Used as the fallback when no per-method override
	// matches. Useful when every Update* method in a service shares the same
	// non-canonical request shape — set the default once instead of registering
	// the same extractor per method.
	//
	// Nil fn is ignored.
	defaultUpdateExtractor extract.UpdateFunc `optgen:"notnil"`
	// WithDefaultReadExtractor replaces the built-in reflection-based read
	// extractor with fn. See [WithDefaultUpdateExtractor].
	defaultReadExtractor extract.ReadFunc `optgen:"notnil"`

	// metadataReadMaskHeader overrides the gRPC metadata key the built-in
	// AIP-157 read extractor consults. Empty means use the default
	// [DefaultMetadataReadMaskHeader]. Populated by
	// [WithMetadataReadMaskHeader].
	// WithMetadataReadMaskHeader preserves whitespace and ignores only the empty string.
	metadataReadMaskHeader string `optgen:"manual"`
	// WithStrictMode enables strict AIP-134 update-mask validation. Update RPCs
	// with a missing or empty mask, or an unknown or malformed path return InvalidArgument with a
	// google.rpc.BadRequest detail; OUTPUT_ONLY paths are dropped from the mask
	// (AIP-203). "*" alone expands to every field but the OUTPUT_ONLY ones.
	// Strict validation is disabled by default.
	strictMode bool `optgen:"default=false"`

	// ignoreMethods is a list of method names to skip entirely. Example:
	// ["/grpc.health.v1.Health/Check"].
	ignoreMethods []string

	// ignorePatterns is a list of regex patterns for methods to skip.
	// Defaults to gRPC reflection and health-check patterns.
	ignorePatterns []*regexp.Regexp `optgen:"default=defaults.IgnorePatterns"`

	// logger is the slog.Logger for debug/error logging. Defaults to a
	// discard logger.
	logger *slog.Logger
}

// WithUpdateExtractor registers a per-method update extractor. fullMethod is
// the value gRPC passes as info.FullMethod ("/x.v1.X/UpdateBucket"). The
// registered function wins over [WithDefaultUpdateExtractor] and the built-in
// reflection extractor when the current method matches.
//
// Use this for requests that do not match the canonical AIP-134 shape — for
// example when the update_mask is nested inside an Options sub-message. The
// generic builder [extract.NewUpdateFunc] reduces boilerplate at the
// call site.
//
//	fieldmask.WithUpdateExtractor("/x.v1.X/UpdateBucket",
//	    extract.NewUpdateFunc(
//	        func(r *pb.UpdateBucketRequest) *fieldmaskpb.FieldMask { return r.GetOptions().GetUpdateMask() },
//	        func(r *pb.UpdateBucketRequest) *pb.Bucket             { return r.GetBucket() },
//	        func(r *pb.UpdateBucketRequest, m *fieldmaskpb.FieldMask) { r.Options.UpdateMask = m },
//	    ),
//	)
//
// Calling the option multiple times accumulates overrides; later calls
// replace earlier ones for the same method. Nil fn is ignored.
func WithUpdateExtractor(fullMethod string, fn extract.UpdateFunc) Option {
	return func(o *options) {
		if fullMethod == "" || fn == nil {
			return
		}

		if o.updateExtractors == nil {
			o.updateExtractors = make(map[string]extract.UpdateFunc)
		}

		o.updateExtractors[fullMethod] = fn
	}
}

// WithReadExtractor registers a per-method read extractor. See
// [WithUpdateExtractor] for the resolution order and the recommended
// [extract.NewReadFunc] builder.
func WithReadExtractor(fullMethod string, fn extract.ReadFunc) Option {
	return func(o *options) {
		if fullMethod == "" || fn == nil {
			return
		}

		if o.readExtractors == nil {
			o.readExtractors = make(map[string]extract.ReadFunc)
		}

		o.readExtractors[fullMethod] = fn
	}
}

// WithMetadataReadMaskHeader overrides the gRPC metadata key the built-in
// AIP-157 read extractor consults. Empty name is ignored and the default
// [DefaultMetadataReadMaskHeader] ("x-goog-fieldmask") is used.
//
// Only the built-in read chain is affected — callers that replace the read
// extractor entirely via [WithDefaultReadExtractor] or [WithReadExtractor]
// own their own header convention.
func WithMetadataReadMaskHeader(name string) Option {
	return func(o *options) {
		if name == "" {
			return
		}

		o.metadataReadMaskHeader = name
	}
}

// WithUpdateSpec registers a handler-owned update spec for fullMethod, such as
// "/example.v1.SettingsService/UpdateSettings". Construct it with
// [update.NewSpec] and [update.WithAllowedPaths]. The factory's unary chain
// resolves and shares this spec with the handler through updatemask.SelectionFromContext.
// The handler owns write planning and reports resolution errors before resource
// body validation. When resolution fails, the downstream Buf validator checks
// only request fields outside the resource and the spec's resource identity fields.
// Nil specs and empty method names are ignored.
func WithUpdateSpec(fullMethod string, spec *update.Spec) Option {
	return func(o *options) {
		if fullMethod == "" || spec == nil {
			return
		}
		if o.updateSpecs == nil {
			o.updateSpecs = make(map[string]*update.Spec)
		}
		o.updateSpecs[fullMethod] = spec
	}
}
