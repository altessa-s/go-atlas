// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package files

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate --type=walkOptions --output=walk_options_gen.go --option-type=WalkOption

import "strings"

// DefaultMaxRecursionDepth is the safety cap applied by [WithRecursive].
// It is large enough for any realistic directory layout but small enough
// to keep a runaway walk (e.g. a symlink cycle reached via
// [WithFollowSymlinks], or a deliberately deep adversarial tree) bounded.
const DefaultMaxRecursionDepth = 256

// walkOptions contains all configurable parameters of [Walk].
//
// Fields tagged optgen:"manual" have hand-written option constructors below
// because they require custom value normalization or aggregation semantics
// that the generator does not provide out of the box.
type walkOptions struct {
	// extensions is a set of file extensions (lowercase, with leading dot).
	// Set via [WithExtensions].
	extensions map[string]struct{} `optgen:"manual"`

	// types is an OR-combined bitmask of [FileType] values.
	// Set via [WithFileTypes]. A zero value matches every type.
	types FileType `optgen:"manual"`

	// maxDepth is the maximum recursion depth. 0 = top-level only,
	// negative = unlimited. Set via [WithMaxDepth] or [WithRecursive].
	maxDepth int

	// skipHidden, when true, skips entries whose name starts with a dot.
	skipHidden bool

	// followSymlinks, when true, instructs [Walk] to descend into symlinked
	// directories.
	followSymlinks bool
}

// WithExtensions filters entries by file extension. Extensions are matched
// case-insensitively against [filepath.Ext]. Each value is normalized to
// start with a leading dot, so both "so" and ".so" are accepted. Empty
// strings (after trimming whitespace) are ignored.
//
// Multiple calls accumulate. Calling WithExtensions with no arguments is a no-op.
func WithExtensions(exts ...string) WalkOption {
	return func(o *walkOptions) {
		// o.extensions is initialized by defaultWalkOptions, so no nil check is needed.
		for _, ext := range exts {
			ext = strings.ToLower(strings.TrimSpace(ext))
			if ext == "" {
				continue
			}
			if !strings.HasPrefix(ext, ".") {
				ext = "." + ext
			}
			o.extensions[ext] = struct{}{}
		}
	}
}

// WithFileTypes filters entries by file type. Multiple calls and multiple
// arguments are OR-combined: an entry passes if its type matches any of the
// supplied types. Pass [FileTypeAny] to match every supported type.
func WithFileTypes(types ...FileType) WalkOption {
	return func(o *walkOptions) {
		for _, t := range types {
			o.types |= t
		}
	}
}

// WithRecursive enables recursion into subdirectories with a safety cap of
// [DefaultMaxRecursionDepth] levels. Use [WithMaxDepth] to set a different
// bound, or [WithUnboundedDepth] to remove the cap entirely.
//
// The safety cap exists because [Walk] does not detect symlink cycles when
// [WithFollowSymlinks] is enabled and because Go's growable stack would
// otherwise allow an adversarial tree to consume unbounded memory.
func WithRecursive() WalkOption {
	return func(o *walkOptions) {
		o.maxDepth = DefaultMaxRecursionDepth
	}
}

// WithUnboundedDepth removes the recursion depth limit entirely.
//
// Use with caution: combined with [WithFollowSymlinks] or applied to an
// untrusted tree, this can lead to an unbounded walk. Prefer [WithRecursive]
// (with the default cap) or [WithMaxDepth] for explicit bounds.
func WithUnboundedDepth() WalkOption {
	return func(o *walkOptions) {
		o.maxDepth = -1
	}
}
