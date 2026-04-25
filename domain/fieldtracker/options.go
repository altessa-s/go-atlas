// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldtracker

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate

const (
	// DefaultMaxDepth is the default maximum recursion depth for nested struct
	// traversal. Set to 0 to disable the depth limit.
	DefaultMaxDepth = 10

	// DefaultTagName is the default struct tag inspected for field names.
	// Override with [WithTagName].
	DefaultTagName = "json"
)

// options holds the configuration for field comparison.
type options struct {
	ignoreFields map[string]struct{} `optgen:"manual,default=make(map[string]struct{})"`
	maxDepth     int                 `optgen:"default=DefaultMaxDepth"`
	tagName      string              `optgen:"default=DefaultTagName"`
}

// WithIgnoreFields specifies field paths to ignore during comparison.
// Field paths use dot notation for nested fields (e.g., "address.city").
func WithIgnoreFields(fields ...string) Option {
	return func(o *options) {
		for _, field := range fields {
			o.ignoreFields[field] = struct{}{}
		}
	}
}
