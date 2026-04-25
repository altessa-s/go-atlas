// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package check

import (
	"fmt"
	"strings"

	"github.com/altessa-s/go-atlas/tools/codegen/optgen/internal/parser"
	"github.com/altessa-s/go-atlas/tools/codegen/optgen/model"
	"github.com/altessa-s/go-atlas/tools/codegen/optgen/plugin"
)

const oneofPriority = 40

// OneOfCheck generates validation code ensuring a string value is one of allowed values.
// Tag usage: optcheck:"oneof=[val1,val2,val3]" where values are string literals.
type OneOfCheck struct {
	plugin.CheckBase
}

func (c *OneOfCheck) Generate(ctx plugin.GenerationContext, field model.OptField, kind, valueVar, rawValue string) []string {
	if kind != KindString || !nonEmpty(rawValue) {
		return nil
	}
	vals := parser.ParseExprList(rawValue)
	if len(vals) == 0 {
		return nil
	}
	return []string{
		"switch " + valueVar + " {",
		"case " + strings.Join(vals, ", ") + ":",
		"default:",
		"  " + buildFail(ctx, fmt.Sprintf("%s has invalid value (allowed: %s)", field.FieldName, strings.Join(vals, ", "))),
		"}",
	}
}

func init() {
	plugin.Register(&OneOfCheck{
		CheckBase: plugin.NewCheckBase("oneof", oneofPriority, plugin.RequiresValue()),
	})
}
