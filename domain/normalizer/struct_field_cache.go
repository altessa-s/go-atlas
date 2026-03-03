// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package normalizer

import (
	"errors"
	"reflect"
	"strings"
	"sync"

	"github.com/altessa-s/go-atlas/domain/normalizer/modifiers"
)

// FieldInfo represents cached field metadata for reflection optimization.
// It stores pre-computed values to avoid repeated reflection operations
// during [Normalize] traversal.
type FieldInfo struct {
	Index           int                        // Field index in struct
	Name            string                     // Field name
	Tag             string                     // Normalize tag value
	IsExported      bool                       // Whether a field is exported
	Kind            reflect.Kind               // Field kind
	IsPointer       bool                       // Whether a field is a pointer
	ElemKind        reflect.Kind               // Element kind for pointers
	ParsedModifiers []modifiers.ModifierParams // Pre-parsed modifier parameters
}

// StructFieldCache represents cached struct field information.
// It holds field metadata and tracks whether the struct implements [CustomNormalizer].
type StructFieldCache struct {
	Fields              []FieldInfo // Field information
	HasCustomNormalizer bool        // Whether struct implements CustomNormalizer
}

// structFieldCacheStorage provides thread-safe storage for struct field metadata.
// Uses RWMutex instead of sync.Map for better read performance in read-heavy workloads.
type structFieldCacheStorage struct {
	mu    sync.RWMutex
	cache map[reflect.Type]*StructFieldCache
}

var (
	// structFieldCache caches struct field metadata for reflection optimization
	structFieldCache = &structFieldCacheStorage{
		cache: make(map[reflect.Type]*StructFieldCache),
	}
)

// GetStructFieldCache retrieves cached struct field information.
// Returns nil if the type is not found in cache.
//
// Example:
//
//	cache := GetStructFieldCache(reflect.TypeOf(User{}))
func GetStructFieldCache(t reflect.Type) *StructFieldCache {
	// Fast path: check cache with read lock
	structFieldCache.mu.RLock()
	cache, found := structFieldCache.cache[t]
	structFieldCache.mu.RUnlock()

	if found {
		return cache
	}
	return nil
}

// SetStructFieldCache stores struct field information in cache.
// The cache is thread-safe and can be accessed concurrently.
func SetStructFieldCache(t reflect.Type, cache *StructFieldCache) {
	structFieldCache.mu.Lock()
	structFieldCache.cache[t] = cache
	structFieldCache.mu.Unlock()
}

// BuildStructFieldCache analyzes a struct type and builds cached field information.
// Returns nil if the type is not a struct. Pre-parses modifier parameters for performance.
//
// Example:
//
//	cache := BuildStructFieldCache(reflect.TypeOf(User{}))
func BuildStructFieldCache(t reflect.Type) *StructFieldCache {
	if t.Kind() != reflect.Struct {
		return nil
	}

	cache := &StructFieldCache{
		Fields: make([]FieldInfo, 0, t.NumField()),
	}

	// Check if struct implements CustomNormalizer interface
	customNormalizerType := reflect.TypeFor[CustomNormalizer]()
	cache.HasCustomNormalizer = t.Implements(customNormalizerType) ||
		reflect.PointerTo(t).Implements(customNormalizerType)

	// Process all fields
	for i := range t.NumField() {
		field := t.Field(i)

		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		tag := field.Tag.Get("normalize")

		// Skip fields with "-" tag (explicitly ignored)
		if tag == "-" {
			continue
		}

		fieldInfo := FieldInfo{
			Index:      i,
			Name:       field.Name,
			Tag:        tag,
			IsExported: field.IsExported(),
			Kind:       field.Type.Kind(),
		}

		// Handle pointer types
		if field.Type.Kind() == reflect.Pointer {
			fieldInfo.IsPointer = true
			fieldInfo.ElemKind = field.Type.Elem().Kind()
		}

		// Pre-parse modifier parameters if tag is not empty and not special cases
		if tag != "" && tag != TagValueCustom {
			modifierStrings := strings.Split(tag, ",")
			fieldInfo.ParsedModifiers = make([]modifiers.ModifierParams, 0, len(modifierStrings))

			for _, modifierStr := range modifierStrings {
				modifierStr = strings.TrimSpace(modifierStr)
				if modifierStr != "" {
					parsed := parseModifierParams(modifierStr)
					fieldInfo.ParsedModifiers = append(fieldInfo.ParsedModifiers, parsed)
				}
			}
		}

		cache.Fields = append(cache.Fields, fieldInfo)
	}

	return cache
}

// ClearStructFieldCache clears the struct field metadata cache.
// Useful for memory management in long-running applications or testing.
//
// Example:
//
//	normalizer.ClearStructFieldCache()
func ClearStructFieldCache() {
	structFieldCache.mu.Lock()
	defer structFieldCache.mu.Unlock()
	clear(structFieldCache.cache)
}

// applyTagModifiers applies pre-parsed modifiers to a field value.
func applyTagModifiers(v reflect.Value, cachedModifiers []modifiers.ModifierParams) error {
	if len(cachedModifiers) == 0 {
		return nil
	}

	// Get modifier chain from pool for better performance
	modifierChain := getModifierChain()
	defer putModifierChain(modifierChain)

	// Pre-allocate capacity for cached modifiers
	if expectedCount := len(cachedModifiers); expectedCount > cap(modifierChain.Entries) {
		modifierChain.Entries = make([]modifierChainEntry, 0, expectedCount)
	}

	for _, parsed := range cachedModifiers {
		// Look up the modifier
		modifier, exists := modifiers.GetModifier(parsed.Name)
		if !exists {
			return errors.New("normalizer: modifier '" + parsed.Name + "' not found")
		}

		entry := modifierChainEntry{
			Name:     parsed.Name,
			Modifier: modifier,
			Params:   parsed.Params,
		}

		modifierChain.Entries = append(modifierChain.Entries, entry)
	}

	// Apply modifiers using the new typed chain
	return applyModifierChainTyped(v, modifierChain)
}
