// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package maps

import "maps"

// Merge creates a new map containing all key-value pairs from both src and dst.
// When a key exists in both maps, the value from src takes precedence over dst.
// Neither input map is modified. If both maps are empty or nil, nil is returned.
//
// Example:
//
//	defaults := map[string]string{"theme": "light", "lang": "en"}
//	userPrefs := map[string]string{"theme": "dark", "timezone": "UTC"}
//	merged := Merge(userPrefs, defaults)
//	// merged is map[string]string{"lang": "en", "theme": "dark", "timezone": "UTC"}
func Merge[K comparable, T any](src map[K]T, dst map[K]T) map[K]T {
	if len(src) == 0 {
		if len(dst) == 0 {
			return nil
		}
		result := make(map[K]T, len(dst))
		maps.Copy(result, dst)
		return result
	}

	if len(dst) == 0 {
		result := make(map[K]T, len(src))
		maps.Copy(result, src)
		return result
	}

	result := make(map[K]T, len(dst)+len(src))
	maps.Copy(result, dst)
	maps.Copy(result, src)
	return result
}

// Swap creates a new map with keys and values transposed from src. If src contains
// duplicate values, only one of the corresponding keys is retained in the result
// (which one is non-deterministic). The original map is not modified.
// Both K and V must be comparable. If src is empty, nil is returned.
//
// Example:
//
//	codes := map[string]int{"error": 500, "success": 200}
//	swapped := Swap(codes) // swapped is map[int]string{200: "success", 500: "error"}
func Swap[K comparable, V comparable](src map[K]V) map[V]K {
	if len(src) == 0 {
		return nil
	}

	result := make(map[V]K, len(src))
	for key, value := range src {
		result[value] = key
	}

	return result
}

// FilterMap returns a new map containing only the key-value pairs from collection
// for which predicate returns true. The original map is not modified.
// If collection is empty or no pairs satisfy the predicate, nil is returned.
//
// Use [Filter] instead when you need a lazy [iter.Seq2] iterator rather than a
// materialized map.
//
// Example:
//
//	ages := map[string]int{"alice": 25, "bob": 17, "charlie": 30}
//	adults := FilterMap(ages, func(name string, age int) bool {
//	    return age >= 18
//	}) // adults is map[string]int{"alice": 25, "charlie": 30}
func FilterMap[K comparable, T any](collection map[K]T, predicate func(K, T) bool) map[K]T {
	if len(collection) == 0 {
		return nil
	}

	result := make(map[K]T, len(collection)/2)
	for key, value := range collection {
		if predicate(key, value) {
			result[key] = value
		}
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

// ConvertMap creates a new map by applying f to each key-value pair in collection,
// producing a new (K2, V2) pair for each entry. Both the key and value types may
// change through the transformation. If f produces duplicate K2 keys, the last
// one wins (order is non-deterministic). The original map is not modified.
// If collection is empty, nil is returned.
//
// Use [Map] instead when you need a lazy [iter.Seq2] iterator rather than a
// materialized map.
//
// Example:
//
//	userIds := map[string]int{"alice": 1, "bob": 2}
//	idToName := ConvertMap(userIds, func(name string, id int) (int, string) {
//	    return id, name
//	}) // idToName is map[int]string{1: "alice", 2: "bob"}
func ConvertMap[K1, K2 comparable, T1, T2 any](collection map[K1]T1, f func(K1, T1) (K2, T2)) map[K2]T2 {
	if len(collection) == 0 {
		return nil
	}

	result := make(map[K2]T2, len(collection))
	for key, value := range collection {
		newKey, newValue := f(key, value)
		result[newKey] = newValue
	}
	return result
}

// FromSlice builds a map from a slice by extracting a key from each element via keyFn.
// The element itself becomes the map value. If keyFn produces duplicate keys, the
// last element with that key wins. If collection is empty, nil is returned.
//
// Use [FromSliceWith] when you need to extract both a custom key and a custom value.
//
// Example:
//
//	type User struct {
//		ID   int
//		Name string
//	}
//	users := []User{{ID: 1, Name: "Alice"}, {ID: 2, Name: "Bob"}}
//	byID := FromSlice(users, func(u User) int { return u.ID })
//	// byID is map[int]User{1: {ID: 1, Name: "Alice"}, 2: {ID: 2, Name: "Bob"}}
func FromSlice[T any, K comparable](collection []T, keyFn func(T) K) map[K]T {
	if len(collection) == 0 {
		return nil
	}

	result := make(map[K]T, len(collection))
	for _, item := range collection {
		result[keyFn(item)] = item
	}

	return result
}

// FromSliceWith builds a map from a slice by applying fn to each element to produce
// both the map key and value. If fn produces duplicate keys, the last element with
// that key wins. If collection is empty, nil is returned.
//
// Use [FromSlice] when the element itself should be the map value and only a key
// extraction function is needed.
//
// Example:
//
//	type User struct {
//		ID   int
//		Name string
//	}
//	users := []User{{ID: 1, Name: "Alice"}, {ID: 2, Name: "Bob"}}
//	names := FromSliceWith(users, func(u User) (int, string) { return u.ID, u.Name })
//	// names is map[int]string{1: "Alice", 2: "Bob"}
func FromSliceWith[T any, K comparable, V any](collection []T, fn func(T) (K, V)) map[K]V {
	if len(collection) == 0 {
		return nil
	}

	result := make(map[K]V, len(collection))
	for _, item := range collection {
		k, v := fn(item)
		result[k] = v
	}

	return result
}

// MergeWith creates a new map containing all key-value pairs from both src and dst.
// When a key exists in both maps, resolve is called with the key and both values
// (src value first, dst value second) to produce the merged result. Neither input
// map is modified. If both maps are empty or nil, nil is returned.
//
// Use [Merge] when source values should unconditionally take precedence. Use
// MergeWith when conflict resolution requires custom logic (e.g. accumulating
// counters or taking the maximum).
//
// Example:
//
//	a := map[string]int{"hits": 3, "misses": 1}
//	b := map[string]int{"hits": 5, "errors": 2}
//	merged := MergeWith(a, b, func(_ string, srcVal, dstVal int) int {
//	    return srcVal + dstVal
//	})
//	// merged is map[string]int{"hits": 8, "misses": 1, "errors": 2}
func MergeWith[K comparable, V any](src, dst map[K]V, resolve func(K, V, V) V) map[K]V {
	if len(src) == 0 {
		if len(dst) == 0 {
			return nil
		}
		result := make(map[K]V, len(dst))
		maps.Copy(result, dst)
		return result
	}
	if len(dst) == 0 {
		result := make(map[K]V, len(src))
		maps.Copy(result, src)
		return result
	}

	result := make(map[K]V, len(dst)+len(src))
	maps.Copy(result, dst)
	for k, srcVal := range src {
		if dstVal, ok := result[k]; ok {
			result[k] = resolve(k, srcVal, dstVal)
		} else {
			result[k] = srcVal
		}
	}
	return result
}

// MergeAll creates a new map by overlaying layers in order: the first layer has
// the lowest priority and the last layer has the highest priority. When the same
// key appears in multiple layers, the value from the rightmost layer wins. All
// input maps are left unmodified. If no layers are provided or all layers are
// empty, nil is returned.
//
// MergeAll(defaults, userConfig, runtimeOverrides) is equivalent to
// Merge(runtimeOverrides, Merge(userConfig, defaults)) but allocates a single
// result map regardless of the number of layers.
//
// Example:
//
//	defaults := map[string]string{"theme": "light", "lang": "en", "timeout": "30s"}
//	user     := map[string]string{"theme": "dark"}
//	runtime  := map[string]string{"timeout": "5s"}
//	result   := MergeAll(defaults, user, runtime)
//	// result is map[string]string{"theme": "dark", "lang": "en", "timeout": "5s"}
func MergeAll[K comparable, V any](layers ...map[K]V) map[K]V {
	totalSize := 0
	for _, l := range layers {
		totalSize += len(l)
	}
	if totalSize == 0 {
		return nil
	}

	result := make(map[K]V, totalSize)
	for _, layer := range layers {
		maps.Copy(result, layer)
	}
	return result
}

// ToKeyValueSlice converts a map to a flat []any containing alternating key, value
// entries (e.g., [k1, v1, k2, v2, ...]). The ordering of pairs is non-deterministic.
// If m is nil or empty, nil is returned.
//
// This is particularly useful for APIs that accept variadic key-value pairs,
// such as slog.Logger.With() or similar structured-logging interfaces.
//
// Example:
//
//	tags := map[string]string{"env": "prod", "region": "us-east"}
//	attrs := ToKeyValueSlice(tags) // attrs is []any{"env", "prod", "region", "us-east"} (order not guaranteed)
//	logger.With(attrs...)
func ToKeyValueSlice[K comparable, V any](m map[K]V) []any {
	if len(m) == 0 {
		return nil
	}

	slice := make([]any, 0, len(m)*2)
	for key, value := range m {
		slice = append(slice, key, value)
	}
	return slice
}
