// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package postprocess

import (
	"strings"

	"github.com/altessa-s/go-atlas/tools/codegen/optgen/internal/plugin/builtin/check"
	"github.com/altessa-s/go-atlas/tools/codegen/optgen/model"
	"github.com/altessa-s/go-atlas/tools/codegen/optgen/plugin"
)

// NonEmptyModifier enforces/normalizes "nonempty" semantics.
//
// Tag usage: optval:"nonempty"
//
// Behavior:
// - string: skips assignment if empty (early return, keeps default/current value)
// - []string: removes empty elements in-place (after any prior transforms)
//
// Rationale: this runs after transformations so trimspaces/lower/etc happen first.
type NonEmptyModifier struct{}

// Key returns the modifier name used in tags.
func (m *NonEmptyModifier) Key() string { return "nonempty" }

// Phase returns PhasePostProcess - runs after transforms.
func (m *NonEmptyModifier) Phase() plugin.Phase { return plugin.PhasePostProcess }

// CanHandle returns true for string and []string fields.
func (m *NonEmptyModifier) CanHandle(field model.OptField) bool {
	return field.Type == check.KindString || (field.IsSlice && field.ElemType == check.KindString)
}

// Generate produces non-empty validation/filtering code.
func (m *NonEmptyModifier) Generate(ctx plugin.GenerationContext, field model.OptField, inputVar string) plugin.ModifierResult {
	if inputVar == "" {
		inputVar = "v"
	}

	// Scalar string: skip assignment if empty (early return).
	if field.Type == check.KindString {
		return plugin.ModifierResult{
			Code:      plugin.BuildEarlyReturn(ctx, inputVar+` == ""`),
			OutputVar: inputVar,
			Final:     true,
		}
	}

	// []string: drop empty entries (in-place).
	if field.IsSlice && field.ElemType == check.KindString {
		tmp := "nonEmpty"
		if strings.EqualFold(inputVar, tmp) {
			tmp = "nonEmpty2"
		}
		code := []string{
			tmp + " := " + inputVar + "[:0]",
			"for _, x := range " + inputVar + " {",
			`  if x != "" {`,
			"    " + tmp + " = append(" + tmp + ", x)",
			"  }",
			"}",
			inputVar + " = " + tmp,
		}
		return plugin.ModifierResult{Code: code, OutputVar: inputVar}
	}

	return plugin.ModifierResult{OutputVar: inputVar}
}

func init() {
	plugin.Register(&NonEmptyModifier{})
}
