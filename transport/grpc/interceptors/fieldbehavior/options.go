// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldbehavior

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate --type=options

import (
	"log/slog"
	"regexp"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/defaults"
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
	// methodKinds overrides ClassifyMethod for specific fully-qualified
	// method names ("/x.v1.X/CreateBucket"). Populated by [WithMethodKind].
	methodKinds map[string]Kind `opt:"-"`

	// skipResponse disables the automatic StripResponse pass on successful
	// responses. Set via [WithSkipResponse]. The per-method KindSkip override
	// is still honored regardless of this flag.
	skipResponse bool `opt:"-"`

	// maxStripDepth caps the descriptor traversal depth passed to every
	// Strip* call. Defaults to DefaultMaxStripDepth.
	maxStripDepth int `optgen:"default=DefaultMaxStripDepth"`

	// ignoreMethods is a list of method names to skip stripping for entirely.
	// Example: ["/grpc.health.v1.Health/Check"]
	ignoreMethods []string

	// ignorePatterns is a list of regex patterns for methods to skip
	// stripping for. Defaults to gRPC reflection and health-check patterns.
	ignorePatterns []*regexp.Regexp `optgen:"default=defaults.IgnorePatterns"`

	// logger is the slog.Logger for debug/error logging. Defaults to a
	// discard logger.
	logger *slog.Logger
}

// WithSkipResponse disables the automatic StripResponse pass that runs on
// every successful response. Use it when the host strips INPUT_ONLY fields
// elsewhere (e.g. inside the handler) and the duplicate walk is wasted work.
func WithSkipResponse() Option {
	return func(o *options) {
		o.skipResponse = true
	}
}

// WithMethodKind overrides the default classification for fullMethod. The key
// is the value gRPC passes as info.FullMethod, including the leading slash
// ("/x.v1.X/CreateBucket"). Calling the option multiple times accumulates
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
