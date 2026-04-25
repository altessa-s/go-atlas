// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package check

import (
	"github.com/altessa-s/go-atlas/tools/codegen/optgen/model"
	"github.com/altessa-s/go-atlas/tools/codegen/optgen/plugin"
)

const minlenPriority = 20

// MinLenCheck generates validation code ensuring a string/slice/map has at least N elements.
// Tag usage: optcheck:"minlen=N" where N is a positive integer.
type MinLenCheck struct {
	plugin.CheckBase
}

func (c *MinLenCheck) Generate(ctx plugin.GenerationContext, field model.OptField, kind, valueVar, rawValue string) []string {
	return buildLenBoundCheck(ctx, field, kind, valueVar, rawValue, "<",
		"%s is too short (min %s)",
		"%s has too few elements (min %s)",
	)
}

func init() {
	plugin.Register(&MinLenCheck{
		CheckBase: plugin.NewCheckBase("minlen", minlenPriority, plugin.RequiresValue()),
	})
}
