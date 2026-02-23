// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package builtin

import (
	"github.com/altessa-s/go-atlas/tools/codegen/optgen/model"
	"github.com/altessa-s/go-atlas/tools/codegen/optgen/plugin"
)

// BuildStringTransformChain builds a chain of string transformations for the given modifiers.
// The varName parameter is the variable name to transform (e.g., "v" or "val").
//
// Example:
//
//	BuildStringTransformChain(ctx, field, "v", []string{"lower", "trimspaces"})
//	// Returns: "strings.TrimSpace(strings.ToLower(v))"
func BuildStringTransformChain(ctx plugin.GenerationContext, field model.OptField, varName string, modifiers []string) (transform string) {
	transform = varName
	for _, mod := range modifiers {
		tp, ok := plugin.FindTransformPlugin(mod, "string")
		if !ok {
			continue
		}
		if next, ok := tp.Apply(ctx, field, "string", transform); ok {
			transform = next
		}
	}
	return transform
}

// HasStringModifier reports whether modifiers contains at least one registered
// string transform plugin (e.g., "lower", "upper", "trimspaces").
func HasStringModifier(modifiers []string) bool {
	for _, mod := range modifiers {
		if _, ok := plugin.FindTransformPlugin(mod, "string"); ok {
			return true
		}
	}
	return false
}
