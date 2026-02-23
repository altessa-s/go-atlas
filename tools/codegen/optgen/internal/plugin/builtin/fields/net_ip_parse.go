// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fields

import (
	"strings"

	"github.com/altessa-s/go-atlas/tools/codegen/optgen/internal/plugin/builtin"
	"github.com/altessa-s/go-atlas/tools/codegen/optgen/model"
	"github.com/altessa-s/go-atlas/tools/codegen/optgen/plugin"
)

const netIPParsePriority = 40

// NetIPParsePlugin generates a With* function that accepts IPs as strings and parses them into []net.IP.
//
// Enable by setting metadata in the tag:
//
//	opt:"FieldName" optgen:"parseip"
//
// Optional metadata:
//   - append: append parsed IPs instead of replacing (default: replace)
//   - notnil: if v is nil (nil slice expansion), do nothing
//   - err=ErrIdent: error expression to return on invalid IP in --option-error mode
//
// The generated option replaces the slice by default.
type NetIPParsePlugin struct{}

func (p *NetIPParsePlugin) Meta() plugin.Meta {
	return plugin.Meta{
		Kind:     plugin.KindField,
		Priority: netIPParsePriority, // higher than slice set/appender
	}
}

func (p *NetIPParsePlugin) CanHandle(field model.OptField) bool {
	if !field.IsSlice || field.ElemType != "net.IP" {
		return false
	}
	return plugin.IsTruthyMetadata(field.Metadata, "parseip")
}

var netIPParseTemplate = builtin.MustOptionTemplate("net-ip-parse",
	`// With{{.OptionName}} {{if .Append}}appends to{{else}}sets{{end}} the {{.FieldName}} option.
func With{{.OptionName}}{{.TypeParamsDecl}}(v ...string) {{.OptionType}}{{.TypeParamsNames}} {
	return func(o *{{.TypeName}}{{.TypeParamsNames}}){{if .OptionReturnsError}} error{{end}} {
{{.GuardsCode}}		netIPs := make([]net.IP, 0, len(v))
		for _, s := range v {
			ip := net.ParseIP(s)
			if ip == nil {
				{{- if .OptionReturnsError}}
				return {{.ErrExpr}}
				{{- else}}
				return
				{{- end}}
			}
			netIPs = append(netIPs, ip)
		}
{{.PostProcessCode}}{{.ChecksCode}}		{{- if .Append}}
		o.{{.FieldName}} = append(o.{{.FieldName}}, netIPs...)
		{{- else}}
		o.{{.FieldName}} = netIPs
		{{- end}}
{{- template "optionReturn" . }}
	}
}`)

func (p *NetIPParsePlugin) Generate(ctx plugin.GenerationContext, field model.OptField) (plugin.GeneratedCode, error) {
	ctx.AddImport("net")

	appendMode := plugin.IsTruthyMetadata(field.Metadata, "append")

	errExpr := `fmt.Errorf("invalid IP %q", s)`
	if v, ok := field.Metadata["err"]; ok {
		v = strings.TrimSpace(v)
		if v != "" {
			errExpr = v
		}
	}
	if ctx.OptionReturnsError && strings.HasPrefix(errExpr, "fmt.") {
		ctx.AddImport("fmt")
	}

	data := struct {
		builtin.OptionData
		Append  bool
		ErrExpr string
	}{
		OptionData: builtin.NewOptionDataWithGuardVar(ctx, field, builtin.CheckKindSlice, "netIPs", "v"),
		Append:     appendMode,
		ErrExpr:    errExpr,
	}
	return builtin.ExecuteOptionTemplate(netIPParseTemplate, data)
}

func init() {
	plugin.Register(&NetIPParsePlugin{})
}
