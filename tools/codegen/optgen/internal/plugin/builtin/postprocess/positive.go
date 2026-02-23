// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package postprocess

import (
	"github.com/altessa-s/go-atlas/tools/codegen/optgen/model"
	"github.com/altessa-s/go-atlas/tools/codegen/optgen/plugin"
)

// PositiveModifier enforces positive value semantics.
//
// Tag usage: optval:"positive" or optval:"positive=allow_zero"
//
// Behavior:
// - Without allow_zero: skips assignment if v <= 0 (only positive values allowed)
// - With allow_zero: skips assignment if v < 0 (zero and positive values allowed)
//
// Works with numeric types: int, int8, int16, int32, int64,
// uint, uint8, uint16, uint32, uint64, float32, float64, time.Duration.
//
// For time.Duration, this modifier is applied by default (can be disabled with optval:"nonpositive").
type PositiveModifier struct{}

// Meta returns plugin metadata.
func (m *PositiveModifier) Meta() plugin.Meta {
	return plugin.Meta{
		Kind:            plugin.KindTransform, // for DefaultForTypes support
		Priority:        5,
		DefaultForTypes: []string{"time.Duration"},
		DisabledBy:      "nonpositive",
	}
}

// Key returns the modifier name used in tags.
func (m *PositiveModifier) Key() string { return "positive" }

// Phase returns PhasePostProcess - runs after transforms.
func (m *PositiveModifier) Phase() plugin.Phase { return plugin.PhasePostProcess }

// numericTypes lists types that support positive check.
var numericTypes = map[string]bool{
	"int": true, "int8": true, "int16": true, "int32": true, "int64": true,
	"uint": true, "uint8": true, "uint16": true, "uint32": true, "uint64": true,
	"float32": true, "float64": true,
	"time.Duration": true,
}

// CanHandle returns true for numeric types.
func (m *PositiveModifier) CanHandle(field model.OptField) bool {
	return numericTypes[field.Type]
}

// Generate produces positive value validation code.
func (m *PositiveModifier) Generate(ctx plugin.GenerationContext, field model.OptField, inputVar string) plugin.ModifierResult {
	if inputVar == "" {
		inputVar = "v"
	}

	// Check if allow_zero is specified
	allowZero := false
	for _, mod := range field.Modifiers {
		if mod == "positive=allow_zero" {
			allowZero = true
			break
		}
	}

	var op string
	if allowZero {
		op = "<" // skip if v < 0 (allow zero and positive)
	} else {
		op = "<=" // skip if v <= 0 (only positive)
	}

	return plugin.ModifierResult{
		Code:      plugin.BuildEarlyReturn(ctx, inputVar+" "+op+" 0"),
		OutputVar: inputVar,
		Final:     true,
	}
}

func init() {
	plugin.Register(&PositiveModifier{})
}
