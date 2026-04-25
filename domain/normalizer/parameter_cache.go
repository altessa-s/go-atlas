// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package normalizer

import (
	"strings"
	"sync"

	"github.com/altessa-s/go-atlas/domain/normalizer/modifiers"
)

// parameterCache holds cached parsed modifier parameters using RWMutex for better read performance
type parameterCache struct {
	mu    sync.RWMutex
	cache map[string]modifiers.ModifierParams
}

var (
	// paramCache caches parsed modifier parameters for performance optimization.
	// Uses RWMutex instead of sync.Map for better read performance in read-heavy workloads.
	paramCache = &parameterCache{
		cache: make(map[string]modifiers.ModifierParams, DefaultParameterCacheCapacity),
	}
)

// ClearParameterCache clears the parameter parsing cache.
// Useful for testing or memory management in long-running applications.
//
// Example:
//
//	normalizer.ClearParameterCache()
func ClearParameterCache() {
	paramCache.mu.Lock()
	defer paramCache.mu.Unlock()
	clear(paramCache.cache)
}

// parseModifierParams parses a modifier string that may contain parameters.
// Uses caching for better performance. Format: "modifier(key1=value1;key2=value2)" or just "modifier".
func parseModifierParams(modifierStr string) modifiers.ModifierParams {
	// Fast path: check cache with read lock
	paramCache.mu.RLock()
	cached, found := paramCache.cache[modifierStr]
	paramCache.mu.RUnlock()

	if found {
		return cached
	}

	// Slow path: parse and cache with write lock
	result := parseModifierParamsUncached(modifierStr)

	paramCache.mu.Lock()
	paramCache.cache[modifierStr] = result
	paramCache.mu.Unlock()

	return result
}

// parseModifierParamsUncached performs the actual parsing without caching.
// This is separated to keep the caching logic clean and testable.
func parseModifierParamsUncached(modifierStr string) modifiers.ModifierParams {
	modifierStr = strings.TrimSpace(modifierStr)

	// Early return for simple case - no parameters
	openParen := strings.IndexByte(modifierStr, '(')
	if openParen == -1 {
		return modifiers.ModifierParams{Name: modifierStr, Params: nil}
	}

	name := strings.TrimSpace(modifierStr[:openParen])

	// Early return for invalid name
	if name == "" {
		return modifiers.ModifierParams{Name: modifierStr, Params: nil}
	}

	// Find closing parenthesis and validate
	closeParen := strings.LastIndexByte(modifierStr, ')')
	if closeParen == -1 || closeParen <= openParen {
		return modifiers.ModifierParams{Name: modifierStr, Params: nil}
	}

	// Extract and process parameter string
	paramStr := strings.TrimSpace(modifierStr[openParen+1 : closeParen])
	if paramStr == "" {
		return modifiers.ModifierParams{Name: name, Params: make(map[string]string)}
	}

	return modifiers.ModifierParams{
		Name:   name,
		Params: parseParameterString(paramStr),
	}
}

// parseParameterString extracts key-value pairs from parameter string
// Format: "key1=value1;key2=value2"
func parseParameterString(paramStr string) map[string]string {
	params := make(map[string]string, DefaultParameterMapCapacity)

	start := 0
	for i := range len(paramStr) + 1 {
		// Process each parameter pair at semicolon or end of string
		if i == len(paramStr) || paramStr[i] == ';' {
			if start < i {
				processPair(params, paramStr[start:i])
			}
			start = i + 1
		}
	}

	return params
}

// processPair adds a single key-value pair to the params map
func processPair(params map[string]string, pair string) {
	pair = strings.TrimSpace(pair)
	if pair == "" {
		return
	}

	equalPos := strings.IndexByte(pair, '=')
	if equalPos == -1 {
		// Parameter without value
		params[pair] = ""
		return
	}

	key := strings.TrimSpace(pair[:equalPos])
	value := strings.TrimSpace(pair[equalPos+1:])
	params[key] = value
}
