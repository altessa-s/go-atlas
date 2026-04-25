// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fields

import (
	"github.com/altessa-s/go-atlas/tools/codegen/optgen/internal/plugin/builtin"
	"github.com/altessa-s/go-atlas/tools/codegen/optgen/model"
	"github.com/altessa-s/go-atlas/tools/codegen/optgen/plugin"
)

// DefaultSetterPlugin generates inline setter for scalar fields.
//
// For non-string fields:
//
//	func WithTimeout(v time.Duration) Option {
//	    return func(o *options) {
//	        o.timeout = v
//	    }
//	}
//
// For string fields (accepts both string and *string, skips empty after transform):
//
//	func WithIssuer[T interface{string|*string}](v T) Option {
//	    return func(o *options) {
//	        switch t := any(v).(type) {
//	        case string:
//	            vv := strings.TrimSpace(t)
//	            if vv == "" { return }
//	            o.issuer = vv
//	        case *string:
//	            if t == nil { return }
//	            vv := strings.TrimSpace(*t)
//	            if vv == "" { return }
//	            o.issuer = vv
//	        }
//	    }
//	}
type DefaultSetterPlugin struct{}

func (p *DefaultSetterPlugin) Meta() plugin.Meta {
	return plugin.Meta{
		Kind:     plugin.KindField,
		Priority: 0, // fallback
	}
}

var setterTemplate = builtin.MustOptionTemplate("setter", `// With{{.OptionName}} sets the {{.FieldName}} option.
func With{{.OptionName}}{{.TypeParamsDecl}}(v {{.Type}}) {{.OptionType}}{{.TypeParamsNames}} {
	return func(o *{{.TypeName}}{{.TypeParamsNames}}){{if .OptionReturnsError}} error{{end}} {
{{.GuardsCode}}{{.TransformCode}}{{.PostProcessCode}}{{.ChecksCode}}		o.{{.FieldName}} = {{.ValueExpr}}
{{- template "optionReturn" . }}
	}
}`)

// stringSetterData contains additional fields for string setter template.
type stringSetterData struct {
	builtin.OptionBaseData
	TransformExpr      string // Transform expression for string value (e.g., "strings.TrimSpace(t)")
	TransformExprDeref string // Transform expression for dereferenced pointer (e.g., "strings.TrimSpace(*t)")
	ChecksCode         string
}

var stringSetterTemplate = builtin.MustOptionTemplate("string_setter", `// With{{.OptionName}} sets the {{.FieldName}} option.
func With{{.OptionName}}{{.TypeParamsDecl}}[T interface{string|*string}](v T) {{.OptionType}}{{.TypeParamsNames}} {
	return func(o *{{.TypeName}}{{.TypeParamsNames}}){{if .OptionReturnsError}} error{{end}} {
		switch t := any(v).(type) {
		case string:
			vv := {{.TransformExpr}}
			if vv == "" {
				return{{if .OptionReturnsError}} nil{{end}}
			}
{{.ChecksCode}}			o.{{.FieldName}} = vv
		case *string:
			if t == nil {
				return{{if .OptionReturnsError}} nil{{end}}
			}
			vv := {{.TransformExprDeref}}
			if vv == "" {
				return{{if .OptionReturnsError}} nil{{end}}
			}
			o.{{.FieldName}} = vv
		}
{{- template "optionReturn" . }}
	}
}`)

func (p *DefaultSetterPlugin) CanHandle(field model.OptField) bool {
	// Handle scalar fields (transforms are applied automatically via TransformCode)
	return !field.IsSlice
}

func (p *DefaultSetterPlugin) Generate(ctx plugin.GenerationContext, field model.OptField) (plugin.GeneratedCode, error) {
	// Use generic template for string fields (but not if struct already has type params)
	if field.Type == "string" && !ctx.IsGeneric() {
		return p.generateStringSetter(ctx, field)
	}

	data := builtin.NewOptionDataForFieldType(ctx, field, "v")
	return builtin.ExecuteOptionTemplate(setterTemplate, data)
}

func (p *DefaultSetterPlugin) generateStringSetter(ctx plugin.GenerationContext, field model.OptField) (plugin.GeneratedCode, error) {
	// Build transform expressions for both cases
	transformExpr := builtin.BuildStringTransformChain(ctx, field, "t", field.Modifiers)
	transformExprDeref := builtin.BuildStringTransformChain(ctx, field, "*t", field.Modifiers)

	// Build checks code referencing the temp variable "vv"
	checksCode := builtin.BuildChecks(ctx, field, builtin.CheckKindString, "vv")

	data := stringSetterData{
		OptionBaseData:     builtin.NewOptionBaseData(ctx, field),
		TransformExpr:      transformExpr,
		TransformExprDeref: transformExprDeref,
		ChecksCode:         checksCode,
	}

	return builtin.ExecuteOptionTemplate(stringSetterTemplate, data)
}

func init() {
	plugin.Register(&DefaultSetterPlugin{})
}
