// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate --type=options

import (
	"log/slog"
	"regexp"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/defaults"

	pbfieldmask "github.com/altessa-s/go-atlas/domain/proto/fieldmask"
)

// Use defaults package for optgen code generation.
var _ = defaults.IgnorePatterns

// options holds configuration for the fieldmask interceptor.
type options struct {
	// methodKinds overrides ClassifyMethod for specific fully-qualified
	// method names ("/x.v1.X/UpdateBucket"). Populated by [WithMethodKind].
	methodKinds map[string]Kind `opt:"-"`

	// skipReadMask disables the response-side Filter pass on KindRead
	// methods. Use it when the handler already applies the read mask and
	// the duplicate walk is wasted work.
	skipReadMask bool `opt:"-"`

	// maskFieldName overrides the conventional update_mask / read_mask
	// field name when extracting from the request. Empty means use the
	// defaults from domain/proto/fieldmask.
	maskFieldName string

	// resourceFieldName overrides the conventional resource carrier field
	// name on Update requests. Empty means "pick the first non-mask
	// message field declared on the request".
	resourceFieldName string

	// updateExtractors holds per-method overrides for locating the
	// update_mask + resource. Populated by [WithUpdateExtractor]. When the
	// current method has an entry it wins over [defaultUpdateExtractor] and
	// the built-in reflection extractor.
	updateExtractors map[string]pbfieldmask.UpdateExtractorFunc `opt:"-"`

	// readExtractors holds per-method overrides for locating the
	// read_mask. Populated by [WithReadExtractor].
	readExtractors map[string]pbfieldmask.ReadExtractorFunc `opt:"-"`

	// defaultUpdateExtractor replaces the built-in reflection-based update
	// extractor when no per-method override matches. Populated by
	// [WithDefaultUpdateExtractor].
	defaultUpdateExtractor pbfieldmask.UpdateExtractorFunc `opt:"-"`

	// defaultReadExtractor replaces the built-in reflection-based read
	// extractor when no per-method override matches. Populated by
	// [WithDefaultReadExtractor].
	defaultReadExtractor pbfieldmask.ReadExtractorFunc `opt:"-"`

	// metadataReadMaskHeader overrides the gRPC metadata key the built-in
	// AIP-157 read extractor consults. Empty means use the default
	// [pbfieldmask.DefaultMetadataReadMaskHeader]. Populated by
	// [WithMetadataReadMaskHeader].
	metadataReadMaskHeader string `opt:"-"`

	// applyEmptyUpdateMask enables the AIP-134 fallback that synthesizes
	// the update_mask from set fields of the resource when the wire
	// update_mask is present but carries zero paths. Off by default to
	// preserve the historical no-op semantics. Populated by
	// [WithApplyEmptyUpdateMask].
	applyEmptyUpdateMask bool `opt:"-"`

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

// WithMethodKind overrides the default classification for fullMethod. The key
// is the value gRPC passes as info.FullMethod, including the leading slash
// ("/x.v1.X/UpdateBucket"). Calling the option multiple times accumulates
// overrides; later calls replace earlier ones for the same method.
func WithMethodKind(fullMethod string, kind Kind) Option {
	return func(o *options) {
		if fullMethod == "" {
			return
		}

		if o.methodKinds == nil {
			o.methodKinds = make(map[string]Kind)
		}

		o.methodKinds[fullMethod] = kind
	}
}

// WithSkipReadMask disables the response-side Filter pass that runs on
// KindRead methods. Use it when the handler already applies the read mask
// itself and the duplicate walk is wasted work.
func WithSkipReadMask() Option {
	return func(o *options) {
		o.skipReadMask = true
	}
}

// WithUpdateExtractor registers a per-method update extractor. fullMethod is
// the value gRPC passes as info.FullMethod ("/x.v1.X/UpdateBucket"). The
// registered function wins over [WithDefaultUpdateExtractor] and the built-in
// reflection extractor when the current method matches.
//
// Use this for requests that do not match the canonical AIP-134 shape — for
// example when the update_mask is nested inside an Options sub-message. The
// generic builder [pbfieldmask.NewUpdateExtractor] reduces boilerplate at the
// call site.
//
//	fieldmask.WithUpdateExtractor("/x.v1.X/UpdateBucket",
//	    pbfieldmask.NewUpdateExtractor(
//	        func(r *pb.UpdateBucketRequest) *fieldmaskpb.FieldMask { return r.GetOptions().GetUpdateMask() },
//	        func(r *pb.UpdateBucketRequest) *pb.Bucket             { return r.GetBucket() },
//	        func(r *pb.UpdateBucketRequest, m *fieldmaskpb.FieldMask) { r.Options.UpdateMask = m },
//	    ),
//	)
//
// Calling the option multiple times accumulates overrides; later calls
// replace earlier ones for the same method. Nil fn is ignored.
func WithUpdateExtractor(fullMethod string, fn pbfieldmask.UpdateExtractorFunc) Option {
	return func(o *options) {
		if fullMethod == "" || fn == nil {
			return
		}

		if o.updateExtractors == nil {
			o.updateExtractors = make(map[string]pbfieldmask.UpdateExtractorFunc)
		}

		o.updateExtractors[fullMethod] = fn
	}
}

// WithReadExtractor registers a per-method read extractor. See
// [WithUpdateExtractor] for the resolution order and the recommended
// [pbfieldmask.NewReadExtractor] builder.
func WithReadExtractor(fullMethod string, fn pbfieldmask.ReadExtractorFunc) Option {
	return func(o *options) {
		if fullMethod == "" || fn == nil {
			return
		}

		if o.readExtractors == nil {
			o.readExtractors = make(map[string]pbfieldmask.ReadExtractorFunc)
		}

		o.readExtractors[fullMethod] = fn
	}
}

// WithDefaultUpdateExtractor replaces the built-in reflection-based update
// extractor with fn. Used as the fallback when no per-method override
// matches. Useful when every Update* method in a service shares the same
// non-canonical request shape — set the default once instead of registering
// the same extractor per method.
//
// Nil fn is ignored.
func WithDefaultUpdateExtractor(fn pbfieldmask.UpdateExtractorFunc) Option {
	return func(o *options) {
		if fn == nil {
			return
		}

		o.defaultUpdateExtractor = fn
	}
}

// WithDefaultReadExtractor replaces the built-in reflection-based read
// extractor with fn. See [WithDefaultUpdateExtractor].
func WithDefaultReadExtractor(fn pbfieldmask.ReadExtractorFunc) Option {
	return func(o *options) {
		if fn == nil {
			return
		}

		o.defaultReadExtractor = fn
	}
}

// WithMetadataReadMaskHeader overrides the gRPC metadata key the built-in
// AIP-157 read extractor consults. Empty name is ignored and the default
// [pbfieldmask.DefaultMetadataReadMaskHeader] ("x-goog-fieldmask") is used.
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

// WithApplyEmptyUpdateMask enables the AIP-134 "empty update_mask updates
// every populated field" fallback on KindUpdate methods.
//
// When the request carries an update_mask field with zero paths AND the
// resource sub-message is present, the interceptor synthesizes a mask via
// [pbfieldmask.FromSetFields] over the resource and applies it normally.
// The synthesized mask is then validated like an explicit one — IMMUTABLE
// and IDENTIFIER fields raise BehaviorViolationError, OUTPUT_ONLY fields
// are stripped before the cleaned mask is written back.
//
// Default behavior is unchanged (an empty mask is a no-op) so existing
// services that rely on it as a deliberate passthrough are not affected.
// The fallback does not fire when the update_mask field is absent
// altogether — that case is still a passthrough.
func WithApplyEmptyUpdateMask() Option {
	return func(o *options) {
		o.applyEmptyUpdateMask = true
	}
}
