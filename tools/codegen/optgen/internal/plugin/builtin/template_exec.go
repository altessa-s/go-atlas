// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package builtin

import (
	"bytes"
	"text/template"

	"github.com/altessa-s/go-atlas/tools/codegen/optgen/plugin"
)

// ExecuteOptionTemplate executes tmpl into a buffer and wraps the output in a
// [plugin.GeneratedCode]. Returns an error if template execution fails.
func ExecuteOptionTemplate(tmpl *template.Template, data any) (plugin.GeneratedCode, error) {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return plugin.GeneratedCode{}, err
	}
	return plugin.GeneratedCode{Code: buf.String()}, nil
}
