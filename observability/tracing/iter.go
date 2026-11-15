// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing

import (
	"iter"

	coreslices "github.com/altessa-s/go-atlas-core/collections/slices"
)

// FilterAttributesByKey returns an iterator that yields attributes with matching keys.
//
// Example:
//
//	for attr := range tracing.FilterAttributesByKey(attrs, "http.") {
//	    // yields attributes starting with "http."
//	}
func FilterAttributesByKey(attrs []Attribute, prefix string) iter.Seq[Attribute] {
	return coreslices.Filter(attrs, func(a Attribute) bool {
		return len(a.Key) >= len(prefix) && a.Key[:len(prefix)] == prefix
	})
}

// AttributeKeys returns an iterator that yields only the keys of attributes.
//
// Example:
//
//	for key := range tracing.AttributeKeys(attrs) {
//	    fmt.Println(key)
//	}
func AttributeKeys(attrs []Attribute) iter.Seq[string] {
	return coreslices.Map(attrs, func(a Attribute) string {
		return a.Key
	})
}
