// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package transform

import (
	"strings"

	"github.com/altessa-s/go-atlas/tools/codegen/optgen/model"
	"github.com/altessa-s/go-atlas/tools/codegen/optgen/plugin"
)

// TrimPrefixTransformPlugin applies strings.TrimPrefix to string expressions.
// Tag usage: optval:"trimprefix=/" or optval:"trimprefix=prefix"
type TrimPrefixTransformPlugin struct {
	plugin.TransformBase
}

func (t *TrimPrefixTransformPlugin) Apply(ctx plugin.GenerationContext, field model.OptField, _ string, expr string) (string, bool) {
	// Find the prefix parameter from field.Modifiers
	prefix := ""
	for _, mod := range field.Modifiers {
		if val, ok := strings.CutPrefix(mod, "trimprefix="); ok {
			prefix = val
			break
		}
	}
	if prefix == "" {
		return expr, false
	}

	ctx.AddImport("strings")
	return "strings.TrimPrefix(" + expr + ", " + quote(prefix) + ")", true
}

// quote returns a Go string literal.
func quote(s string) string {
	return `"` + escapeString(s) + `"`
}

// escapeString escapes special characters in a string for Go string literal.
func escapeString(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func init() {
	plugin.Register(&TrimPrefixTransformPlugin{
		TransformBase: plugin.NewTransformBase("trimprefix", plugin.ForTypes("string")),
	})
}
