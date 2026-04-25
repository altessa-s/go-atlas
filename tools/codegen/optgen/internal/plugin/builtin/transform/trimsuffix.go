// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package transform

import (
	"strings"

	"github.com/altessa-s/go-atlas/tools/codegen/optgen/model"
	"github.com/altessa-s/go-atlas/tools/codegen/optgen/plugin"
)

// TrimSuffixTransformPlugin applies strings.TrimSuffix to string expressions.
// Tag usage: optval:"trimsuffix=/" or optval:"trimsuffix=suffix"
type TrimSuffixTransformPlugin struct {
	plugin.TransformBase
}

func (t *TrimSuffixTransformPlugin) Apply(ctx plugin.GenerationContext, field model.OptField, _ string, expr string) (string, bool) {
	// Find the suffix parameter from field.Modifiers
	suffix := ""
	for _, mod := range field.Modifiers {
		if val, ok := strings.CutPrefix(mod, "trimsuffix="); ok {
			suffix = val
			break
		}
	}
	if suffix == "" {
		return expr, false
	}

	ctx.AddImport("strings")
	return "strings.TrimSuffix(" + expr + ", " + quote(suffix) + ")", true
}

func init() {
	plugin.Register(&TrimSuffixTransformPlugin{
		TransformBase: plugin.NewTransformBase("trimsuffix", plugin.ForTypes("string")),
	})
}
