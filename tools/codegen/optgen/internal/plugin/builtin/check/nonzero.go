// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package check

import (
	"fmt"

	"github.com/altessa-s/go-atlas/tools/codegen/optgen/model"
	"github.com/altessa-s/go-atlas/tools/codegen/optgen/plugin"
)

const nonzeroPriority = 90

// NonZeroCheck generates validation code ensuring a numeric value is non-zero.
// Supports: int, int8-64, uint, uint8-64, uintptr, float32, float64, time.Duration.
// Tag usage: optcheck:"nonzero"
type NonZeroCheck struct {
	plugin.CheckBase
}

func (c *NonZeroCheck) Generate(ctx plugin.GenerationContext, field model.OptField, kind, valueVar, _ string) []string {
	if kind != KindOther {
		return nil
	}
	switch field.Type {
	case "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64", "uintptr",
		"float32", "float64",
		"time.Duration":
		return []string{
			"if " + valueVar + " == 0 {",
			"  " + buildFail(ctx, fmt.Sprintf("%s must be non-zero", field.FieldName)),
			"}",
		}
	default:
		return nil
	}
}

func init() {
	plugin.Register(&NonZeroCheck{
		CheckBase: plugin.NewCheckBase("nonzero", nonzeroPriority),
	})
}
