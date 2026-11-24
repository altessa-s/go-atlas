// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing

import (
	"slices"

	coreslices "github.com/altessa-s/go-atlas-core/collections/slices"
)

// Attribute represents a key-value pair attached to a span.
// Attributes provide context about the operation being traced.
// Custom reference values must remain immutable through export.
type Attribute struct {
	Key   string
	Value any
}

// attributesPool provides pooling for Attributes slices.
var attributesPool = coreslices.NewPool[Attribute](16) //nolint:mnd // Pool capacity hint

// GetAttributes retrieves an Attributes slice from the pool.
// Use PutAttributes to return it to the pool after use.
func GetAttributes() *[]Attribute {
	return attributesPool.Get()
}

// GetAttributesWithCapacity retrieves an Attributes slice from the pool with the given capacity.
func GetAttributesWithCapacity(capacity int) *[]Attribute {
	return attributesPool.GetWithCapacity(capacity)
}

// PutAttributes returns an Attributes slice to the pool for reuse.
func PutAttributes(attrs *[]Attribute) {
	attributesPool.Put(attrs)
}

// String creates a string attribute.
func String(key, value string) Attribute {
	return Attribute{Key: key, Value: value}
}

// Int creates an int attribute.
func Int(key string, value int) Attribute {
	return Attribute{Key: key, Value: value}
}

// Int64 creates an int64 attribute.
func Int64(key string, value int64) Attribute {
	return Attribute{Key: key, Value: value}
}

// Float64 creates a float64 attribute.
func Float64(key string, value float64) Attribute {
	return Attribute{Key: key, Value: value}
}

// Bool creates a bool attribute.
func Bool(key string, value bool) Attribute {
	return Attribute{Key: key, Value: value}
}

// StringSlice creates a string slice attribute.
func StringSlice(key string, value []string) Attribute {
	return Attribute{Key: key, Value: slices.Clone(value)}
}

// IntSlice creates an int slice attribute.
func IntSlice(key string, value []int) Attribute {
	return Attribute{Key: key, Value: slices.Clone(value)}
}

// Int64Slice creates an int64 slice attribute.
func Int64Slice(key string, value []int64) Attribute {
	return Attribute{Key: key, Value: slices.Clone(value)}
}

// Float64Slice creates a float64 slice attribute.
func Float64Slice(key string, value []float64) Attribute {
	return Attribute{Key: key, Value: slices.Clone(value)}
}

// BoolSlice creates a bool slice attribute.
func BoolSlice(key string, value []bool) Attribute {
	return Attribute{Key: key, Value: slices.Clone(value)}
}

// Valid returns true if the attribute has a non-empty key.
func (a Attribute) Valid() bool {
	return a.Key != ""
}

// snapshotValue copies mutable values supported by the typed attribute API at
// span end, so callers may reuse their slices while a batch is in flight.
func snapshotValue(value any) any {
	switch v := value.(type) {
	case []string:
		return slices.Clone(v)
	case []int:
		return slices.Clone(v)
	case []int64:
		return slices.Clone(v)
	case []float64:
		return slices.Clone(v)
	case []bool:
		return slices.Clone(v)
	case []byte:
		return slices.Clone(v)
	default:
		return v
	}
}
