// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package check

import (
	"github.com/altessa-s/go-atlas/tools/codegen/optgen/model"
	"github.com/altessa-s/go-atlas/tools/codegen/optgen/plugin"
)

// MaxLenCheck generates validation code ensuring a string/slice/map has at most N elements.
// Tag usage: optcheck:"maxlen=N" where N is a positive integer.
type MaxLenCheck struct {
	plugin.CheckBase
}

func (c *MaxLenCheck) Generate(ctx plugin.GenerationContext, field model.OptField, kind, valueVar, rawValue string) []string {
	return buildLenBoundCheck(ctx, field, kind, valueVar, rawValue, ">",
		"%s is too long (max %s)",
		"%s has too many elements (max %s)",
	)
}

func init() {
	plugin.Register(&MaxLenCheck{
		CheckBase: plugin.NewCheckBase("maxlen", 30, plugin.RequiresValue()),
	})
}
