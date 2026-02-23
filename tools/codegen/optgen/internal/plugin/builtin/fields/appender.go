// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fields

import (
	"github.com/altessa-s/go-atlas/tools/codegen/optgen/internal/plugin/builtin"
	"github.com/altessa-s/go-atlas/tools/codegen/optgen/model"
	"github.com/altessa-s/go-atlas/tools/codegen/optgen/plugin"
)

// DefaultAppenderPlugin generates inline appender for slice fields without modifiers.
//
// Generated code example:
//
//	func WithPaths(v ...string) Option {
//	    return func(o *options) {
//	        o.paths = append(o.paths, v...)
//	    }
//	}
type DefaultAppenderPlugin struct{}

func (p *DefaultAppenderPlugin) Meta() plugin.Meta {
	return plugin.Meta{
		Kind:     plugin.KindField,
		Priority: 0, // fallback
	}
}

var appenderTemplate = builtin.MustOptionTemplate("appender", `// With{{.OptionName}} appends to the {{.FieldName}} option.
func With{{.OptionName}}{{.TypeParamsDecl}}(v ...{{.ElemType}}) {{.OptionType}}{{.TypeParamsNames}} {
	return func(o *{{.TypeName}}{{.TypeParamsNames}}){{if .OptionReturnsError}} error{{end}} {
{{.GuardsCode}}{{.TransformCode}}{{.PostProcessCode}}{{.ChecksCode}}		o.{{.FieldName}} = append(o.{{.FieldName}}, {{.ValueExpr}}...)
{{- template "optionReturn" . }}
	}
}`)

func (p *DefaultAppenderPlugin) CanHandle(field model.OptField) bool {
	// Handle slice fields when append semantics are explicitly requested.
	// Transforms are applied automatically via TransformCode.
	if !field.IsSlice {
		return false
	}
	return plugin.IsTruthyMetadata(field.Metadata, "append")
}

func (p *DefaultAppenderPlugin) Generate(ctx plugin.GenerationContext, field model.OptField) (plugin.GeneratedCode, error) {
	data := builtin.NewOptionData(ctx, field, builtin.CheckKindSlice, "v")
	return builtin.ExecuteOptionTemplate(appenderTemplate, data)
}

func init() {
	plugin.Register(&DefaultAppenderPlugin{})
}
