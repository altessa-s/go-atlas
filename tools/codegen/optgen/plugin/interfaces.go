// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package plugin

import (
	"reflect"

	"github.com/altessa-s/go-atlas/tools/codegen/optgen/model"
)

// GetPluginName returns the display name for a plugin. If the plugin
// implements [PluginMeta] and Meta().Name is non-empty, that name is used;
// otherwise the concrete type name is obtained via reflection (dereferencing
// one pointer level if needed).
func GetPluginName(p any) string {
	if pm, ok := p.(PluginMeta); ok {
		if name := pm.Meta().Name; name != "" {
			return name
		}
	}
	t := reflect.TypeOf(p)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.Name()
}

// FieldPlugin is the primary plugin type: it selects and generates the
// WithXxx option function for a struct field. The [Registry] evaluates
// plugins in descending priority order and uses the first one whose
// CanHandle returns true.
type FieldPlugin interface {
	PluginMeta
	// CanHandle determines if this plugin can generate code for the field.
	// Called for each field during generation to find the appropriate plugin.
	//
	// Return true if this plugin should handle the field, false otherwise.
	// The registry will use the first plugin that returns true, sorted by priority.
	CanHandle(field model.OptField) bool

	// Generate generates the option function code for the field.
	// This is called after CanHandle returns true.
	Generate(ctx GenerationContext, field model.OptField) (GeneratedCode, error)
}

// TransformPlugin applies a single optval-style modifier to an expression.
//
// Transform plugins are looked up by Key() (e.g. "lower", "trimspaces") and applied
// in tag order to build a transformation chain.
//
// Use Meta.ApplicableTypes to restrict transforms to specific types (e.g. "string").
// When a transform is used for a non-applicable type, it is ignored and a warning
// is emitted (if a warning sink is configured).
type TransformPlugin interface {
	PluginMeta
	// Key is the modifier token used in optval/optgen modifiers (e.g. "lower").
	Key() string
	// Apply transforms expr and returns the new expression.
	//
	// typeStr is the type of expr (this can differ from field.Type, e.g. slice element transforms).
	Apply(ctx GenerationContext, field model.OptField, typeStr, expr string) (newExpr string, ok bool)
}

// GeneratedCode is the output of a [FieldPlugin.Generate] call. Code is the
// primary WithXxx function body; TypeDefs and Helpers carry supplementary
// declarations that the generator places before and after option functions.
type GeneratedCode struct {
	// Code is the main generated code (e.g., the WithXxx function).
	Code string

	// TypeDefs is a list of additional type definitions needed.
	// These will be placed before the option functions.
	TypeDefs []string

	// Helpers is a list of additional helper functions needed.
	// These will be placed after the option functions.
	Helpers []string
}

// TypeDefaultPlugin provides a default-value expression for a given type
// string (e.g. "*slog.Logger" -> "slog.Default()"). It is consulted when no
// explicit default is specified in the optgen tag. Plugins are tried in
// descending priority order.
type TypeDefaultPlugin interface {
	PluginMeta
	// CanProvideDefault determines if this provider can supply a default value
	// for the given type string (e.g., "*slog.Logger", "time.Duration").
	CanProvideDefault(typeStr string) bool

	// GetDefault returns the default value expression for the given type.
	GetDefault(typeStr string) string
}
