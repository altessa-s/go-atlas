// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fields

import (
	"github.com/altessa-s/go-atlas/tools/codegen/optgen/internal/plugin/builtin"
	"github.com/altessa-s/go-atlas/tools/codegen/optgen/model"
	"github.com/altessa-s/go-atlas/tools/codegen/optgen/plugin"
)

const boolFlagPriority = 20

// BoolFlagPlugin generates WithXxx() or WithoutXxx() for bool fields.
//
// Default behavior (no modifiers):
//
//	func WithEnabled() Option {
//	    return func(o *options) {
//	        o.enabled = true
//	    }
//	}
//
// With optval:"invert":
//
//	func WithoutValidation() Option {
//	    return func(o *options) {
//	        o.skipValidation = true
//	    }
//	}
//
// With optval:"param":
//
//	func WithEnforceMandatory(v bool) Option {
//	    return func(o *options) {
//	        o.enforceMandatory = v
//	    }
//	}
type BoolFlagPlugin struct{}

func (p *BoolFlagPlugin) Meta() plugin.Meta {
	return plugin.Meta{
		Kind:     plugin.KindField,
		Priority: boolFlagPriority,
	}
}

type boolFlagData struct {
	builtin.OptionBaseData
	FuncPrefix  string
	CommentVerb string
}

var boolFlagTemplate = builtin.MustOptionTemplate("bool_flag", `// {{.FuncPrefix}}{{.OptionName}} {{.CommentVerb}} the {{.FieldName}} option.
func {{.FuncPrefix}}{{.OptionName}}{{.TypeParamsDecl}}() {{.OptionType}}{{.TypeParamsNames}} {
	return func(o *{{.TypeName}}{{.TypeParamsNames}}){{if .OptionReturnsError}} error{{end}} {
		o.{{.FieldName}} = true
{{- template "optionReturn" . }}
	}
}`)

var boolSetterTemplate = builtin.MustOptionTemplate("bool_setter", `// With{{.OptionName}} sets the {{.FieldName}} option.
func With{{.OptionName}}{{.TypeParamsDecl}}(v bool) {{.OptionType}}{{.TypeParamsNames}} {
	return func(o *{{.TypeName}}{{.TypeParamsNames}}){{if .OptionReturnsError}} error{{end}} {
		o.{{.FieldName}} = v
{{- template "optionReturn" . }}
	}
}`)

func (p *BoolFlagPlugin) CanHandle(field model.OptField) bool {
	return field.Type == "bool" && !field.IsSlice
}

func (p *BoolFlagPlugin) Generate(ctx plugin.GenerationContext, field model.OptField) (plugin.GeneratedCode, error) {
	if field.HasModifier("param") {
		data := builtin.NewOptionBaseData(ctx, field)
		return builtin.ExecuteOptionTemplate(boolSetterTemplate, data)
	}

	// Check for invert modifier
	invert := field.HasModifier("invert")

	data := boolFlagData{
		OptionBaseData: builtin.NewOptionBaseData(ctx, field),
	}

	if invert {
		data.FuncPrefix = "Without"
		data.CommentVerb = "disables"
	} else {
		data.FuncPrefix = "With"
		data.CommentVerb = "enables"
	}

	return builtin.ExecuteOptionTemplate(boolFlagTemplate, data)
}

func init() {
	plugin.Register(&BoolFlagPlugin{})
}
