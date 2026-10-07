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
			for field := range strings.FieldsSeq(t) {
				if !yield(field) {
					return
				}
			}
		case []string:
			for _, s := range t {
				if !yield(s) {
					return
				}
			}
		case []any:
			for _, raw := range t {
				if s, ok := raw.(string); ok {
					if !yield(s) {
						return
					}
				}
			}
		}
	}
}
