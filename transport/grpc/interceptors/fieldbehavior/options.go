// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldbehavior

//go:generate go tool optgen generate --type=options

import (
	"log/slog"
	"regexp"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/defaults"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/updatemask"
)

// Use defaults package for optgen code generation.
var _ = defaults.IgnorePatterns

// DefaultMaxStripDepth bounds the descriptor traversal depth passed to every
// Strip* call. Mirrors fieldbehavior.DefaultMaxDepth so that the interceptor
// behaves identically to direct fieldbehavior usage when no override is
// supplied.
const DefaultMaxStripDepth = 32

// options holds configuration for the fieldbehavior interceptor.
type options struct {
	// WithMethodKind overrides the default classification for fullMethod. The key
	// is the value gRPC passes as info.FullMethod, including the leading slash
	// ("/x.v1.X/CreateBucket"). Calling the option multiple times accumulates
	// overrides; later calls replace earlier ones for the same method.
	methodKinds map[string]Kind `opt:"MethodKind" optgen:"put"`
	// WithSkipResponse disables the automatic StripResponse pass that runs on
	// every successful response. Use it when the host strips INPUT_ONLY fields
	// elsewhere (e.g. inside the handler) and the duplicate walk is wasted work.
	skipResponse bool `optgen:"default=false"`

	// maxStripDepth caps the descriptor traversal depth passed to every
	// Strip* call. Defaults to DefaultMaxStripDepth.
	maxStripDepth int `optgen:"default=DefaultMaxStripDepth"`

	// ignoreMethods is a list of method names to skip stripping for entirely.
	// Example: ["/grpc.health.v1.Health/Check"]
	ignoreMethods []string

	// ignorePatterns is a list of regex patterns for methods to skip
	// stripping for. Defaults to gRPC reflection and health-check patterns.
	ignorePatterns []*regexp.Regexp `optgen:"default=defaults.IgnorePatterns"`

	// WithUpdateNormalizer sets how the update_mask of an Update request is
	// applied to its resource. Defaults to updatemask.New(): the canonical
	// AIP-134 request shape, the default update options. Give it the
	// normalizer of the fieldmask interceptor (fieldmask.UpdateNormalizer with
	// the same options) so both apply a mask the same way: a renamed or nested
	// mask field, per-method extractors, mask limits, strict mode. Nil is
	// ignored.
	// By default, the singular google.protobuf.FieldMask is declared as update_mask,
	// falling back to updateMask only when update_mask is not declared.
	updateNormalizer *updatemask.Normalizer `optgen:"notnil"`

	// logger is the slog.Logger for debug/error logging. Defaults to a
	// discard logger.
	logger *slog.Logger
}
