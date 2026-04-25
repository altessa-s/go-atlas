// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fields

import (
	"github.com/altessa-s/go-atlas/tools/codegen/optgen/internal/plugin/builtin"
	"github.com/altessa-s/go-atlas/tools/codegen/optgen/model"
	"github.com/altessa-s/go-atlas/tools/codegen/optgen/plugin"
)

const sliceSetPriority = 35

// SliceSetPlugin generates "setter" semantics for slice fields (replace, not append).
//
// Default behavior for slices is "set" (no extra tag parameters required).
// To get append semantics, use:
//
//	opt:"FieldName" optgen:"append"
type SliceSetPlugin struct{}

func (p *SliceSetPlugin) Meta() plugin.Meta {
	return plugin.Meta{
		Kind:     plugin.KindField,
		Priority: sliceSetPriority, // higher than default appender
	}
}

func (p *SliceSetPlugin) CanHandle(field model.OptField) bool {
	if !field.IsSlice {
		return false
	}
	// If append is requested, do not handle (let appender plugin take it).
	if plugin.IsTruthyMetadata(field.Metadata, "append") {
		return false
	}
	// Transforms are applied automatically via TransformCode.
	return true
}

var sliceSetTemplate = builtin.MustOptionTemplate("slice-set", `// With{{.OptionName}} sets the {{.FieldName}} option.
func With{{.OptionName}}{{.TypeParamsDecl}}(v ...{{.ElemType}}) {{.OptionType}}{{.TypeParamsNames}} {
	return func(o *{{.TypeName}}{{.TypeParamsNames}}){{if .OptionReturnsError}} error{{end}} {
{{.GuardsCode}}{{.TransformCode}}{{.PostProcessCode}}{{.ChecksCode}}		o.{{.FieldName}} = {{.ValueExpr}}
{{- template "optionReturn" . }}
	}
}`)

func (p *SliceSetPlugin) Generate(ctx plugin.GenerationContext, field model.OptField) (plugin.GeneratedCode, error) {
	data := builtin.NewOptionData(ctx, field, builtin.CheckKindSlice, "v")
	return builtin.ExecuteOptionTemplate(sliceSetTemplate, data)
}

func init() {
	plugin.Register(&SliceSetPlugin{})
}
