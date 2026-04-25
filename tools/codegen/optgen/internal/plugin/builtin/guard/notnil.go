// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package guard

import (
	"strings"

	"github.com/altessa-s/go-atlas/tools/codegen/optgen/model"
	"github.com/altessa-s/go-atlas/tools/codegen/optgen/plugin"
)

const notNilGuardPriority = 100

// NotNilModifier implements optgen:"notnil" semantics as a pipeline guard.
//
// If the option input is nil, the option becomes a no-op and keeps defaults.
// This is useful for pointers/maps/interfaces and for variadic slice options invoked as xs... where xs is nil.
//
// Tag usage: optgen:"notnil"
//
// This modifier is applied by default to all pointer and interface types.
// Use optgen:"allownull" to disable this behavior for a specific field.
type NotNilModifier struct{}

// Meta returns plugin metadata.
func (m *NotNilModifier) Meta() plugin.Meta {
	return plugin.Meta{
		Kind:            plugin.KindTransform, // for DefaultForTypes support
		Priority:        notNilGuardPriority,  // run first in guards
		DefaultForTypes: []string{"*", "interface", "[]"},
		DisabledBy:      "allownull",
	}
}

// Key returns the modifier name used in tags.
func (m *NotNilModifier) Key() string { return "notnil" }

// Phase returns PhaseGuard - this runs first in the pipeline.
func (m *NotNilModifier) Phase() plugin.Phase { return plugin.PhaseGuard }

// CanHandle returns true for nilable types (pointers, slices, maps, interfaces, channels, functions).
func (m *NotNilModifier) CanHandle(field model.OptField) bool {
	// If user explicitly requested notnil via tag, trust them
	if _, explicit := field.Metadata["notnil"]; explicit {
		return true
	}
	// Use IsNilable if set, otherwise fallback to type string checks
	if field.IsNilable || field.IsInterface {
		return true
	}
	// Fallback for backwards compatibility
	return strings.HasPrefix(field.Type, "*") || field.IsSlice || strings.HasPrefix(field.Type, "map[")
}

// Generate produces early-return code if input is nil (or empty for slices).
// For interface types, uses nilcheck.IsNil to correctly detect nil interface values
// wrapping nil concrete pointers (a common Go pitfall where v == nil is false
// but the underlying value is nil).
func (m *NotNilModifier) Generate(ctx plugin.GenerationContext, field model.OptField, inputVar string) plugin.ModifierResult {
	if inputVar == "" {
		inputVar = "v"
	}

	var condition string
	switch {
	case field.IsSlice:
		condition = "len(" + inputVar + ") == 0"
	case isInterfaceField(field):
		ctx.AddImport("github.com/altessa-s/go-atlas/core/types/nilcheck")
		condition = "nilcheck.IsNil(" + inputVar + ")"
	default:
		condition = inputVar + " == nil"
	}

	return plugin.ModifierResult{
		Code:      plugin.BuildEarlyReturn(ctx, condition),
		OutputVar: inputVar,
		Final:     true, // Stop pipeline if nil/empty
	}
}

// isInterfaceField returns true if the field is an interface type.
// Handles both AST-detected interfaces (field.IsInterface) and named interface types
// that the AST parser cannot resolve without go/types (e.g. metrics.Collector).
// Named types that are not pointers, slices, maps, channels, or functions are
// assumed to be interfaces when the field has "notnil" metadata.
func isInterfaceField(field model.OptField) bool {
	if field.IsInterface {
		return true
	}
	// If the field is explicitly tagged as notnil but is not a recognized concrete nilable
	// type (pointer, slice, map, chan, func), it is most likely a named interface.
	if _, explicit := field.Metadata["notnil"]; explicit {
		return !field.IsSlice && !field.IsNilable && !strings.HasPrefix(field.Type, "*") && !strings.HasPrefix(field.Type, "map[")
	}
	return false
}

func init() {
	plugin.Register(&NotNilModifier{})
}
