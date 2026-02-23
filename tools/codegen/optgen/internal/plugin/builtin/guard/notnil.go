// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package guard

import (
	"strings"

	"github.com/altessa-s/go-atlas/tools/codegen/optgen/model"
	"github.com/altessa-s/go-atlas/tools/codegen/optgen/plugin"
)

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
		Priority:        100,                  // run first in guards
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
func (m *NotNilModifier) Generate(ctx plugin.GenerationContext, field model.OptField, inputVar string) plugin.ModifierResult {
	if inputVar == "" {
		inputVar = "v"
	}

	condition := inputVar + " == nil"
	if field.IsSlice {
		condition = "len(" + inputVar + ") == 0"
	}

	return plugin.ModifierResult{
		Code:      plugin.BuildEarlyReturn(ctx, condition),
		OutputVar: inputVar,
		Final:     true, // Stop pipeline if nil/empty
	}
}

func init() {
	plugin.Register(&NotNilModifier{})
}
