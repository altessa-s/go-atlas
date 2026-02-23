// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package transform

import (
	"github.com/altessa-s/go-atlas/tools/codegen/optgen/model"
	"github.com/altessa-s/go-atlas/tools/codegen/optgen/plugin"
)

// TrimSpacesTransformPlugin applies strings.TrimSpace to string expressions.
// It is the default transform for all string fields; use optval:"notrim" to
// disable it. Tag usage: optval:"trimspaces"
type TrimSpacesTransformPlugin struct {
	plugin.TransformBase
}

func (t *TrimSpacesTransformPlugin) Apply(ctx plugin.GenerationContext, _ model.OptField, _ string, expr string) (string, bool) {
	ctx.AddImport("strings")
	return "strings.TrimSpace(" + expr + ")", true
}

func init() {
	plugin.Register(&TrimSpacesTransformPlugin{
		TransformBase: plugin.NewTransformBase("trimspaces",
			plugin.ForTypes("string"),
			plugin.DefaultFor("string"),
			plugin.DisabledBy("notrim"),
		),
	})
}
