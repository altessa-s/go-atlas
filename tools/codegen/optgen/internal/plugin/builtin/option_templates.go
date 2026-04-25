// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package builtin

import "text/template"

// OptionTemplateCommon is a text/template fragment that every field-handler
// template includes. It defines the "optionReturn" block that emits
// "return nil" when the option function signature returns an error.
const OptionTemplateCommon = `
{{- define "optionReturn" -}}
{{- if .OptionReturnsError}}
		return nil
{{- end}}
{{- end}}`

// MustOptionTemplate parses body prepended with [OptionTemplateCommon] and panics
// on error. The returned template is ready to execute with [OptionData] or a
// superset thereof.
func MustOptionTemplate(name, body string) *template.Template {
	return template.Must(template.New(name).Parse(OptionTemplateCommon + body))
}
