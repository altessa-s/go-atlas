// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package check

import (
	"fmt"

	"github.com/altessa-s/go-atlas/tools/codegen/optgen/model"
	"github.com/altessa-s/go-atlas/tools/codegen/optgen/plugin"
)

// RequiredCheck generates validation code ensuring a field value is non-empty/non-nil.
// Supports: pointers (nil check), strings (empty check), slices/maps (nil or empty check).
// Tag usage: optcheck:"required" or optcheck:"nonempty" (alias)
type RequiredCheck struct {
	plugin.CheckBase
}

func (c *RequiredCheck) Generate(ctx plugin.GenerationContext, field model.OptField, kind, valueVar, _ string) []string {
	if kind == "pointer" {
		return []string{
			"if " + valueVar + " == nil {",
			"  " + buildFail(ctx, field, fmt.Sprintf("%s is required", field.FieldName)),
			"}",
		}
	}
	if kind == "string" {
		return []string{
			`if ` + valueVar + ` == "" {`,
			"  " + buildFail(ctx, field, fmt.Sprintf("%s is required", field.FieldName)),
			"}",
		}
	}
	if kind == "slice" || kind == "map" {
		return []string{
			"if " + valueVar + " == nil || len(" + valueVar + ") == 0 {",
			"  " + buildFail(ctx, field, fmt.Sprintf("%s is required", field.FieldName)),
			"}",
		}
	}
	return nil
}

func init() {
	plugin.Register(&RequiredCheck{
		CheckBase: plugin.NewCheckBase("required", 10, plugin.WithAliases("nonempty")),
	})
}
