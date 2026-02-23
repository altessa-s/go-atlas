// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package transform

import (
	"github.com/altessa-s/go-atlas/tools/codegen/optgen/model"
	"github.com/altessa-s/go-atlas/tools/codegen/optgen/plugin"
)

// UpperTransformPlugin applies strings.ToUpper to string expressions.
// Tag usage: optval:"upper"
type UpperTransformPlugin struct {
	plugin.TransformBase
}

func (t *UpperTransformPlugin) Apply(ctx plugin.GenerationContext, _ model.OptField, _ string, expr string) (string, bool) {
	ctx.AddImport("strings")
	return "strings.ToUpper(" + expr + ")", true
}

func init() {
	plugin.Register(&UpperTransformPlugin{
		TransformBase: plugin.NewTransformBase("upper", plugin.ForTypes("string")),
	})
}
