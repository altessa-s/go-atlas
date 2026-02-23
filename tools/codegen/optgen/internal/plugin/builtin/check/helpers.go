// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package check

import (
	"fmt"
	"strings"

	"github.com/altessa-s/go-atlas/tools/codegen/optgen/model"
	"github.com/altessa-s/go-atlas/tools/codegen/optgen/plugin"
)

// Kind name constants used by check plugins to classify field types.
const (
	KindString  = "string"
	KindSlice   = "slice"
	KindMap     = "map"
	KindPointer = "pointer"
	KindOther   = "other"
)

func buildFail(ctx plugin.GenerationContext, msg string) string {
	// Always use fmt.Errorf so we can include context; in non-error mode we panic.
	ctx.AddImport("fmt")
	errExpr := fmt.Sprintf(`fmt.Errorf(%q)`, msg)
	if ctx.OptionReturnsError {
		return "return " + errExpr
	}
	return "panic(" + errExpr + ")"
}

func nonEmpty(s string) bool { return strings.TrimSpace(s) != "" }

func buildLenBoundCheck(
	ctx plugin.GenerationContext,
	field model.OptField,
	kind string,
	valueVar string,
	rawValue string,
	op string,
	stringMsgFmt string,
	collectionMsgFmt string,
) []string {
	if kind != KindString && kind != KindSlice && kind != KindMap {
		return nil
	}
	if !nonEmpty(rawValue) {
		return nil
	}

	limit := strings.TrimSpace(rawValue)
	msg := fmt.Sprintf(stringMsgFmt, field.FieldName, limit)
	if kind == KindSlice || kind == KindMap {
		msg = fmt.Sprintf(collectionMsgFmt, field.FieldName, limit)
	}

	return []string{
		"if len(" + valueVar + ") " + op + " (" + rawValue + ") {",
		"  " + buildFail(ctx, msg),
		"}",
	}
}
