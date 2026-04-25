// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package maps

import (
	"strings"
)

// ConflictHandler is a callback invoked by [FromFlatMapWithHandler] when a dot-separated
// key path attempts to traverse through a value that is not a map[string]any.
// The key parameter is the full dot-separated key being processed, and existingValue
// is the non-map value that blocks further traversal. The conflicting entry is
// skipped regardless of what the handler does; the handler exists for observability
// (e.g., logging or counting conflicts).
type ConflictHandler func(key string, existingValue any)

// FromFlatMap converts a flat map with dot-separated keys (e.g., "parent.child.key")
// into a nested map[string]any hierarchy. It is a shorthand for calling
// [FromFlatMapWithHandler] with a nil [ConflictHandler].
//
// If a key path conflicts with an existing non-map value, the conflicting entry is
// silently skipped. Use [FromFlatMapWithHandler] to observe such conflicts.
// If the input map is empty, nil is returned.
//
// Example:
//
//	flat := map[string]any{
//	    "user.name": "Alice",
//	    "user.age": 25,
//	    "user.address.city": "NYC",
//	}
//	nested := FromFlatMap(flat)
//	// nested is map[string]any{"user": map[string]any{"name": "Alice", "age": 25, "address": map[string]any{"city": "NYC"}}}
func FromFlatMap(in map[string]any) map[string]any {
	return FromFlatMapWithHandler(in, nil)
}

// FromFlatMapWithHandler converts a flat map with dot-separated keys into a nested
// map[string]any hierarchy, invoking onConflict when a key path cannot be expanded
// because an intermediate segment already holds a non-map value.
//
// The [ConflictHandler] is optional; pass nil to silently skip conflicts (equivalent
// to [FromFlatMap]). Conflicting entries are always skipped regardless of handler behavior.
// If the input map is empty, nil is returned.
//
// Example:
//
//	conflicts := 0
//	handler := func(key string, existingValue any) {
//	    log.Printf("Conflict at key %s with value %v", key, existingValue)
//	    conflicts++
//	}
//	nested := FromFlatMapWithHandler(flat, handler)
//	fmt.Printf("Encountered %d conflicts\n", conflicts)
func FromFlatMapWithHandler(in map[string]any, onConflict ConflictHandler) map[string]any {
	if len(in) == 0 {
		return nil
	}

	out := make(map[string]any)
	for key, value := range in {
		// Optimization: if no dot, just set and continue
		dotIdx := strings.IndexByte(key, '.')
		if dotIdx < 0 {
			out[key] = value
			continue
		}

		current := out
		remaining := key
		for {
			dotIdx = strings.IndexByte(remaining, '.')
			if dotIdx < 0 {
				// Last segment
				current[remaining] = value
				break
			}

			segment := remaining[:dotIdx]
			remaining = remaining[dotIdx+1:]

			var next map[string]any
			if existing, exists := current[segment]; exists {
				var ok bool
				next, ok = existing.(map[string]any)
				if !ok {
					// Type conflict
					if onConflict != nil {
						onConflict(key, existing)
					}
					// Skip this key
					goto nextKey
				}
			} else {
				next = make(map[string]any)
				current[segment] = next
			}
			current = next
		}
	nextKey:
	}

	return out
}

// ToFlatMap converts a nested map[string]any hierarchy into a flat map whose keys
// are dot-joined paths representing the position of each leaf value. This is the
// inverse of [FromFlatMap].
//
// The optional keyMapper transforms individual key segments before they are joined;
// pass nil to use segments as-is. Only leaf values (non-map[string]any) appear in the
// output. If the input map is empty, nil is returned.
//
// Example:
//
//	nested := map[string]any{
//	    "user": map[string]any{
//	        "name": "Alice",
//	        "address": map[string]any{"city": "NYC"},
//	    },
//	}
//	flat := ToFlatMap(nested, nil)
//	// flat is map[string]any{"user.name": "Alice", "user.address.city": "NYC"}
func ToFlatMap(in map[string]any, keyMapper func(key string) string) map[string]any {
	if len(in) == 0 {
		return nil
	}

	if keyMapper == nil {
		keyMapper = func(key string) string { return key }
	}

	out := make(map[string]any)
	prefix := ""
	flattenNested(in, out, prefix, keyMapper)
	return out
}

func flattenNested(in map[string]any, out map[string]any, prefix string, keyMapper func(key string) string) {
	for key, value := range in {
		mappedKey := keyMapper(key)
		fullKey := mappedKey
		if prefix != "" {
			fullKey = prefix + "." + mappedKey
		}

		if nestedMap, ok := value.(map[string]any); ok {
			flattenNested(nestedMap, out, fullKey, keyMapper)
		} else {
			out[fullKey] = value
		}
	}
}
