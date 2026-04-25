// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing

import (
	"iter"

	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
)

// Attributes returns an iterator over the slice of attributes.
// Use slices.Collect() to materialize the result into a slice.
//
// Example:
//
//	attrs := []Attribute{String("k1", "v1"), Int("k2", 42)}
//	for attr := range tracing.Attributes(attrs) {
//	    fmt.Println(attr.Key, attr.Value)
//	}
func Attributes(attrs []Attribute) iter.Seq[Attribute] {
	return coreslices.Values(attrs)
}

// Links returns an iterator over the slice of links.
// Use slices.Collect() to materialize the result into a slice.
//
// Example:
//
//	for link := range tracing.Links(links) {
//	    fmt.Println(link.SpanContext.TraceID())
//	}
func Links(links []Link) iter.Seq[Link] {
	return coreslices.Values(links)
}

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

// MapAttributes returns an iterator that applies a transformation function to each attribute.
//
// Example:
//
//	keys := slices.Collect(tracing.MapAttributes(attrs, func(a Attribute) string {
//	    return a.Key
//	}))
func MapAttributes[O any](attrs []Attribute, fn func(Attribute) O) iter.Seq[O] {
	return coreslices.Map(attrs, fn)
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
