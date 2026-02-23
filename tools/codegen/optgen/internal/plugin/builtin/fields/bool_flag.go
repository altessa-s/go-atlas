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

func (p *BoolFlagPlugin) CanHandle(field model.OptField) bool {
	return field.Type == "bool" && !field.IsSlice
}

func (p *BoolFlagPlugin) Generate(ctx plugin.GenerationContext, field model.OptField) (plugin.GeneratedCode, error) {
	// Check for invert modifier
	invert := false
	for _, mod := range field.Modifiers {
		if mod == "invert" {
			invert = true
			break
		}
	}

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
