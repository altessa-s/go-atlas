// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filter

import (
	"regexp"
	"slices"
	"strings"

	"github.com/altessa-s/go-atlas/core/runtime/panics"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// TextSearch returns a [CustomFunction] for a free-text search over fields:
// name("text") expands into
//
//	field1.matches(re) || field2.matches(re) || ...
//
// where re is "(?i)" followed by the trimmed text quoted with
// [regexp.QuoteMeta], so the search is case-insensitive and literal (no
// wildcards, no anchors) and every translator check — allowlist, field
// mapping, regex validation — applies to the expansion unchanged. It
// combines with the rest of a filter like any predicate:
//
//	filter.WithCustomFunctions(map[string]filter.CustomFunction{
//	    "search": filter.TextSearch("code", "name", "description"),
//	})
//	// search("network") && lawCode == "44fz"
//
// The argument must be one non-blank string literal; anything else fails
// with [ErrInvalidExpression]. Field names are API names (before
// [WithFieldMapping]). TextSearch panics when called without fields.
//
// No full-text index is used: on MongoDB each field becomes an unanchored
// case-insensitive $regex, which scans. Translators escape what their engine
// cannot carry (the MongoDB translator spells a NUL byte \x00).
func TextSearch(fields ...string) CustomFunction {
	panics.Must(len(fields) > 0, "filter: TextSearch needs at least one field")
	fields = slices.Clone(fields)
	return func(args []Node) (Node, error) {
		if len(args) != 1 {
			return nil, coreerrs.Wrap(ErrInvalidExpression, "text search takes one string argument")
		}
		lit, ok := args[0].(*LiteralNode)
		if !ok {
			return nil, coreerrs.Wrap(ErrInvalidExpression, "text search takes a string literal")
		}
		text, ok := lit.Value.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return nil, coreerrs.Wrap(ErrInvalidExpression, "text search takes a non-empty string")
		}
		pattern := "(?i)" + regexp.QuoteMeta(strings.TrimSpace(text))
		var out Node
		for _, f := range fields {
			m := &CallNode{Op: OpMatches, Target: &IdentNode{Name: f}, Args: []Node{&LiteralNode{Value: pattern}}}
			if out == nil {
				out = m
				continue
			}
			out = &BinaryOpNode{Op: OpOr, Left: out, Right: m}
		}
		return out, nil
	}
}
