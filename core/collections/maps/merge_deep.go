// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package maps

// MergeDeep recursively merges src into dst, returning a new map[string]any that
// contains all keys from both maps. When a key exists in both maps and both values
// are themselves map[string]any, the function recurses into them; otherwise src
// values take precedence, matching the shallow behavior of [Merge]. Neither input
// is modified; all nested maps in the result are independent deep copies.
// If both maps are empty or nil, nil is returned.
//
// MergeDeep is the idiomatic choice for overlaying YAML/JSON config layers where
// base settings must be preserved unless explicitly overridden at any nested depth.
//
// Example:
//
//	base := map[string]any{
//	    "db":  map[string]any{"host": "localhost", "port": 5432, "timeout": 10},
//	    "log": "info",
//	}
//	override := map[string]any{
//	    "db": map[string]any{"host": "prod.db"},
//	}
//	merged := MergeDeep(override, base)
//	// merged is map[string]any{
//	//     "db":  map[string]any{"host": "prod.db", "port": 5432, "timeout": 10},
//	//     "log": "info",
//	// }
func MergeDeep(src, dst map[string]any) map[string]any {
	if len(src) == 0 && len(dst) == 0 {
		return nil
	}
	if len(src) == 0 {
		return deepCopyMap(dst)
	}
	if len(dst) == 0 {
		return deepCopyMap(src)
	}

	result := make(map[string]any, len(dst)+len(src))

	// Copy dst-only entries; deep-copy any nested maps so the result is independent.
	for k, v := range dst {
		if _, inSrc := src[k]; inSrc {
			continue // handled in the src pass below
		}
		if nested, ok := v.(map[string]any); ok {
			result[k] = deepCopyMap(nested)
		} else {
			result[k] = v
		}
	}

	// Apply src entries; recursively merge when both sides hold nested maps.
	for k, srcVal := range src {
		dstVal, inDst := dst[k]
		srcMap, srcIsMap := srcVal.(map[string]any)
		if inDst {
			dstMap, dstIsMap := dstVal.(map[string]any)
			if srcIsMap && dstIsMap {
				result[k] = MergeDeep(srcMap, dstMap)
				continue
			}
		}
		if srcIsMap {
			result[k] = deepCopyMap(srcMap)
		} else {
			result[k] = srcVal
		}
	}

	return result
}

// deepCopyMap returns a new map[string]any with all nested map[string]any values
// recursively copied. Non-map values are assigned by value (shallow for slice/pointer types).
func deepCopyMap(m map[string]any) map[string]any {
	result := make(map[string]any, len(m))
	for k, v := range m {
		if nested, ok := v.(map[string]any); ok {
			result[k] = deepCopyMap(nested)
		} else {
			result[k] = v
		}
	}
	return result
}
