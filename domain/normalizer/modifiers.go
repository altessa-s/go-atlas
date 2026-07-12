// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package normalizer

import (
	"reflect"
	"sync"

	"github.com/altessa-s/go-atlas/domain/normalizer/modifiers"
)

const (
	// DefaultModifierChainCapacity is the default capacity for modifier chains.
	// Most fields have 1-3 modifiers, so 4 provides good balance.
	DefaultModifierChainCapacity = 4

	// DefaultParameterMapCapacity is the default capacity for parameter maps.
	// Most parameterized modifiers have 1-2 parameters.
	DefaultParameterMapCapacity = 2

	// DefaultParameterCacheCapacity is the default capacity for parameter cache.
	// Most applications use 10-50 unique modifier strings.
	DefaultParameterCacheCapacity = 32
)

// CustomNormalizer interface allows types to implement their own normalization logic.
// The Normalize() method is called after all registered modifiers have been applied.
//
// Example:
//
//	func (u *User) Normalize() error { u.Email = strings.ToLower(u.Email); return nil }
type CustomNormalizer interface {
	Normalize() error
}

// modifierChainEntry represents a single entry in the modifier chain
type modifierChainEntry struct {
	Name     string             // Modifier name for debugging
	Modifier modifiers.Modifier // The modifier function (handles both simple and parameterized)
	Params   map[string]string  // Parameters for modifiers (can be nil if not needed)
}

// modifierChain represents a chain of modifiers to be applied
type modifierChain struct {
	Entries []modifierChainEntry // The actual modifier entries
}

var (
	// modifierChainPool provides memory pooling for modifierChain instances
	modifierChainPool = sync.Pool{
		New: func() any {
			return &modifierChain{
				Entries: make([]modifierChainEntry, 0, DefaultModifierChainCapacity),
			}
		},
	}
)

// getModifierChain returns a modifierChain from the memory pool.
func getModifierChain() *modifierChain {
	chain, ok := modifierChainPool.Get().(*modifierChain)
	if !ok {
		return &modifierChain{
			Entries: make([]modifierChainEntry, 0, DefaultModifierChainCapacity),
		}
	}
	return chain
}

// putModifierChain returns a modifierChain to the memory pool for reuse.
func putModifierChain(chain *modifierChain) {
	if chain != nil {
		// Clear the entries (dropping references to modifier closures and
		// param maps) before truncating so the pooled backing array does not
		// pin them for the pool's lifetime, then keep the capacity.
		clear(chain.Entries)
		chain.Entries = chain.Entries[:0]
		modifierChainPool.Put(chain)
	}
}

// applyModifierChainTyped applies a typed modifier chain to a reflect.Value.
// This version eliminates any boxing and provides better performance.
func applyModifierChainTyped(v reflect.Value, modifierChain *modifierChain) error {
	if len(modifierChain.Entries) == 0 {
		return nil
	}

	// Apply modifiers directly to the reflect.Value
	current := v
	for _, entry := range modifierChain.Entries {
		if entry.Modifier == nil {
			// No modifier set, skip
			continue
		}

		// Apply the modifier with parameters (nil params are fine)
		result := entry.Modifier(current, entry.Params)

		// Stop on error (propagate).
		if result.Error != nil {
			// Best-effort: ensure modifier name is set if missing.
			if result.Error.ModifierName == "" {
				result.Error.ModifierName = entry.Name
			}
			return result.Error
		}

		// Update the value if the result is valid and can be set
		if result.Value.IsValid() && current.CanSet() {
			// Handle type compatibility
			if result.Value.Type().AssignableTo(current.Type()) {
				current.Set(result.Value)
			}
		}
	}

	return nil
}
