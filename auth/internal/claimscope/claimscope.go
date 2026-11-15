// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package claimscope

import (
	"iter"
	"strings"
)

// Seq yields the scopes carried by a scope claim value, splitting a
// space-separated string or iterating an array. Non-string array elements
// and unsupported types are skipped.
func Seq(v any) iter.Seq[string] {
	return func(yield func(string) bool) {
		switch t := v.(type) {
		case string:
			yieldFields(t, yield)
		case []string:
			yieldAll(t, yield)
		case []any:
			yieldStrings(t, yield)
		}
	}
}

// yieldFields yields the space-separated fields of s until yield stops.
func yieldFields(s string, yield func(string) bool) {
	for field := range strings.FieldsSeq(s) {
		if !yield(field) {
			return
		}
	}
}

// yieldAll yields every element of t until yield stops.
func yieldAll(t []string, yield func(string) bool) {
	for _, s := range t {
		if !yield(s) {
			return
		}
	}
}

// yieldStrings yields the string elements of t, skipping the others, until
// yield stops.
func yieldStrings(t []any, yield func(string) bool) {
	for _, raw := range t {
		if s, ok := raw.(string); ok && !yield(s) {
			return
		}
	}
}
