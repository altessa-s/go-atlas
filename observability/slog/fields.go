// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package slog

import (
	"context"
	"iter"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
)

// FieldKey is the type alias for field keys in logs.
type FieldKey = string

// Field is a key-value pair for structured log fields.
type Field struct {
	Key   FieldKey
	Value any
}

// Fields is an ordered collection of [Field] values.
// It provides methods for manipulation and conversion to [slog.Attr].
type Fields []Field

// Append appends a field to the fields.
func (f Fields) Append(key FieldKey, value any) Fields {
	return append(f, Field{Key: key, Value: value})
}

// AppendFields appends fields to the fields.
func (f Fields) AppendFields(fields Fields) Fields {
	return append(f, fields...)
}

// Delete removes the first field with the given key. Like [slices.Delete], it
// shifts elements within the receiver's backing array and zeroes the vacated
// tail slot (so the removed value is not retained); use the returned slice.
func (f Fields) Delete(key FieldKey) Fields {
	for i := range f {
		if f[i].Key == key {
			return slices.Delete(f, i, i+1)
		}
	}
	return f
}

// Unique returns a new Fields with unique keys.
// If there are duplicate keys, the first occurrence is kept.
func (f Fields) Unique() Fields {
	return coreslices.DeduplicateBy(f, func(field Field) FieldKey {
		return field.Key
	})
}

// All returns an iterator over all fields as key-value pairs.
// This allows using Fields with Go 1.23+ range over function syntax.
//
// Example:
//
//	for key, value := range fields.All() {
//	    fmt.Printf("%s: %v\n", key, value)
//	}
func (f Fields) All() iter.Seq2[FieldKey, any] {
	return func(yield func(FieldKey, any) bool) {
		for _, field := range f {
			if !yield(field.Key, field.Value) {
				return
			}
		}
	}
}

// ToSlogArgs converts Fields to slog key-value args for Logger.With().
//
// Example:
//
//	logger.With(fields.ToSlogArgs()...)
func (f Fields) ToSlogArgs() []any {
	return coreslices.Reduce(f, make([]any, 0, len(f)*2), func(acc []any, _ int, field Field) []any {
		return append(acc, field.Key, field.Value)
	})
}

// ToSlogAttrs converts Fields to slog.Attr slice for LogAttrs().
func (f Fields) ToSlogAttrs() []slog.Attr {
	return slices.Collect(coreslices.Map(f, func(field Field) slog.Attr {
		return slog.Any(field.Key, field.Value)
	}))
}

// fieldsWrapper wraps Fields with a mutex for thread-safe access.
type fieldsWrapper struct {
	mu     sync.RWMutex
	fields Fields
}

type fieldsContextKey struct{}

// FieldsFromContext returns a copy of the [Fields] stored via [InjectFields].
// Returns nil if no fields have been injected.
// Safe for concurrent use.
func FieldsFromContext(ctx context.Context) Fields {
	fw, ok := ctx.Value(fieldsContextKey{}).(*fieldsWrapper)
	if !ok || fw == nil {
		return nil
	}

	fw.mu.RLock()
	n := make(Fields, len(fw.fields))
	copy(n, fw.fields)
	fw.mu.RUnlock()

	return n
}

// InjectFields stores fields in the context for later retrieval via [FieldsFromContext].
// Subsequent calls to [AppendField] and [AppendFields] mutate the stored fields.
//
// The wrapper is allocated fresh per context and deliberately not pooled: it is
// request-scoped and lives as long as the context, so returning it to a sync.Pool
// while the context still references it would let a later request observe or
// overwrite another request's fields (a cross-request data leak). The previous
// pool was also never returned, so it provided no reuse regardless.
func InjectFields(ctx context.Context, f Fields) context.Context {
	fw := &fieldsWrapper{fields: append(make(Fields, 0, len(f)), f...)}
	return context.WithValue(ctx, fieldsContextKey{}, fw)
}

// AppendField appends a field to the [Fields] previously stored via [InjectFields].
// No-op if no fields have been injected. Safe for concurrent use.
func AppendField(ctx context.Context, key FieldKey, value any) {
	fw, ok := ctx.Value(fieldsContextKey{}).(*fieldsWrapper)
	if !ok || fw == nil {
		return
	}
	fw.mu.Lock()
	fw.fields = fw.fields.Append(key, value)
	fw.mu.Unlock()
}

// AppendFields appends multiple fields to the [Fields] previously stored via [InjectFields].
// No-op if no fields have been injected. Safe for concurrent use.
func AppendFields(ctx context.Context, f Fields) {
	fw, ok := ctx.Value(fieldsContextKey{}).(*fieldsWrapper)
	if !ok || fw == nil {
		return
	}
	fw.mu.Lock()
	fw.fields = fw.fields.AppendFields(f)
	fw.mu.Unlock()
}

// FieldsToAttrs converts logging fields to slog attributes with safe grouping
// by dot-separated keys. Order is deterministic (keys are sorted at each level).
func FieldsToAttrs(fields Fields) []slog.Attr {
	if len(fields) == 0 {
		return nil
	}

	root := make(map[string]any, len(fields))

	for _, f := range fields {
		key := f.Key
		idx := strings.IndexByte(key, '.')
		if idx <= 0 {
			// keep first value if duplicate key appears
			if _, exists := root[key]; !exists {
				root[key] = f.Value
			}
			continue
		}

		parts := strings.Split(key, ".")
		current := root
		for i := range len(parts) - 1 {
			part := parts[i]
			next, ok := current[part]
			if !ok {
				child := make(map[string]any)
				current[part] = child
				current = child
				continue
			}
			child, ok := next.(map[string]any)
			if !ok {
				// replace conflicting non-map with a new map to avoid panics
				child = make(map[string]any)
				current[part] = child
			}
			current = child
		}

		last := parts[len(parts)-1]
		if _, exists := current[last]; !exists {
			current[last] = f.Value
		}
	}

	return mapToAttrs(root)
}

func mapToAttrs(data map[string]any) []slog.Attr {
	keys := slices.Sorted(maps.Keys(data))

	return slices.Collect(coreslices.Map(keys, func(k string) slog.Attr {
		v := data[k]
		switch typed := v.(type) {
		case map[string]any:
			return groupAttr(k, typed)
		default:
			return valueToAttr(k, typed)
		}
	}))
}

func groupAttr(key string, data map[string]any) slog.Attr {
	children := mapToAttrs(data)

	// slog.Group expects a flat list of Attr or key/value pairs.
	args := slices.Collect(coreslices.Map(children, func(attr slog.Attr) any {
		return attr
	}))
	return slog.Group(key, args...)
}

func valueToAttr(key string, v any) slog.Attr {
	switch value := v.(type) {
	case string:
		return slog.String(key, value)
	case int:
		return slog.Int(key, value)
	case int8:
		return slog.Int(key, int(value))
	case int16:
		return slog.Int(key, int(value))
	case int32:
		return slog.Int(key, int(value))
	case int64:
		return slog.Int64(key, value)
	case uint:
		return slog.Uint64(key, uint64(value))
	case uint8:
		return slog.Uint64(key, uint64(value))
	case uint16:
		return slog.Uint64(key, uint64(value))
	case uint32:
		return slog.Uint64(key, uint64(value))
	case uint64:
		return slog.Uint64(key, value)
	case float32:
		return slog.Float64(key, float64(value))
	case float64:
		return slog.Float64(key, value)
	case bool:
		return slog.Bool(key, value)
	case time.Duration:
		return slog.Duration(key, value)
	case time.Time:
		return slog.Time(key, value)
	default:
		return slog.Any(key, value)
	}
}
