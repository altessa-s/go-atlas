// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fields

import (
	"github.com/altessa-s/go-atlas/tools/codegen/optgen/model"
	"github.com/altessa-s/go-atlas/tools/codegen/optgen/plugin"
)

// ManualPlugin skips generating a With* function for a field while still allowing
// the field to participate in defaultOptions/newOptions generation.
//
// Use by setting optgen metadata:
//
//	opt:"FieldName" optgen:"manual"
//
// Or by using opt:"-" with optgen:"default=...":
//
//	opt:"-" optgen:"default=SomeDefaultValue()"
//
// This is useful when a field needs a custom With* implementation (validation,
// normalization, cross-field behavior) but you still want defaults/newOptions
// generated consistently.
type ManualPlugin struct{}

func (p *ManualPlugin) Meta() plugin.Meta {
	return plugin.Meta{
		Kind:     plugin.KindField,
		Priority: 110, // explicit user request should override all other plugins
	}
}

func (p *ManualPlugin) CanHandle(field model.OptField) bool {
	// Handle explicit "manual" metadata or DefaultOnly fields (opt:"-" with default)
	return plugin.IsTruthyMetadata(field.Metadata, "manual") || field.DefaultOnly
}

func (p *ManualPlugin) Generate(_ plugin.GenerationContext, _ model.OptField) (plugin.GeneratedCode, error) {
	// Intentionally generate no With* function.
	return plugin.GeneratedCode{Code: ""}, nil
}

func init() {
	plugin.Register(&ManualPlugin{})
}
