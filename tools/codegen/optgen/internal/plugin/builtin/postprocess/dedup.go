// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package postprocess

import (
	"fmt"
	"strings"

	"github.com/altessa-s/go-atlas/tools/codegen/optgen/internal/plugin/builtin/check"
	"github.com/altessa-s/go-atlas/tools/codegen/optgen/model"
	"github.com/altessa-s/go-atlas/tools/codegen/optgen/plugin"
)

const dedupPriority = 5

// DedupModifier applies deduplication to slice values.
// It is applied by default to slices of comparable types (primitives, pointers).
//
// Tag usage: optval:"dedup" (explicit) or automatic for comparable slices
// To disable: optval:"nodup"
//
// Generated code:
//
//	v = slices.Deduplicate(v)
type DedupModifier struct{}

// Meta returns plugin metadata including default application for slices.
func (m *DedupModifier) Meta() plugin.Meta {
	return plugin.Meta{
		Kind:            plugin.KindTransform, // Used for DefaultForTypes lookup
		Priority:        dedupPriority,
		DefaultForTypes: []string{"[]"}, // Apply to all slice types (filtered by CanHandle)
		DisabledBy:      "nodup",
	}
}

// Key returns the modifier name used in tags.
func (m *DedupModifier) Key() string { return "dedup" }

// Phase returns PhasePostProcess - runs after transforms.
func (m *DedupModifier) Phase() plugin.Phase { return plugin.PhasePostProcess }

// CanHandle returns true for slice fields with comparable element types.
// Deduplication requires comparable types for slices.Deduplicate to work.
func (m *DedupModifier) CanHandle(field model.OptField) bool {
	if !field.IsSlice {
		return false
	}

	// Only handle slices with comparable element types
	return isComparableType(field.ElemType)
}

// isComparableType checks if a type is known to be comparable.
// Returns true for primitive types, pointers, and common comparable types.
func isComparableType(typeStr string) bool {
	// Pointer types are always comparable
	if strings.HasPrefix(typeStr, "*") {
		return true
	}

	// Check for known comparable primitive types
	switch typeStr {
	case check.KindString, "bool", "byte", "rune", "error",
		"int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64", "uintptr",
		"float32", "float64",
		"complex64", "complex128":
		return true
	}

	// Common comparable types from standard library
	switch typeStr {
	case "time.Time", "time.Duration":
		return true
	}

	return false
}

// Generate produces deduplication code.
func (m *DedupModifier) Generate(ctx plugin.GenerationContext, _ model.OptField, inputVar string) plugin.ModifierResult {
	ctx.AddImport("github.com/altessa-s/go-atlas/core/collections/slices")
	return plugin.ModifierResult{
		Code: []string{
			fmt.Sprintf("%s = slices.Deduplicate(%s)", inputVar, inputVar),
		},
		OutputVar: inputVar,
	}
}

func init() {
	plugin.Register(&DedupModifier{})
}
