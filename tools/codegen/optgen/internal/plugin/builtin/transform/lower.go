// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package transform

import (
	"github.com/altessa-s/go-atlas/tools/codegen/optgen/model"
	"github.com/altessa-s/go-atlas/tools/codegen/optgen/plugin"
)

// LowerTransformPlugin applies strings.ToLower to string expressions.
// Tag usage: optval:"lower"
type LowerTransformPlugin struct {
	plugin.TransformBase
}

func (t *LowerTransformPlugin) Apply(ctx plugin.GenerationContext, _ model.OptField, _ string, expr string) (string, bool) {
	ctx.AddImport("strings")
	return "strings.ToLower(" + expr + ")", true
}

func init() {
	plugin.Register(&LowerTransformPlugin{
		TransformBase: plugin.NewTransformBase("lower", plugin.ForTypes("string")),
	})
}
