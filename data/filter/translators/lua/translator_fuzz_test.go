// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package lua_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/filter"
	"github.com/altessa-s/go-atlas/data/filter/translators/lua"
)

var allowedFields = []string{"name", "age", "active", "created_at", "tags", "score"}

var filterSeeds = []string{
	`name == "John"`,
	`age > 18 && active == true`,
	`name.contains("oh")`,
	`name in ["a", "b"]`,
	`!active`,
	`name == "\""`,
	`name == "\\"`,
	`name == "\" or true or \""`,
	`name == "]]"`,
	`name.startsWith("\\")`,
	`unknown_field == "x"`,
	``,
}

func newTranslator(tb testing.TB) *lua.Translator {
	tb.Helper()

	tr, err := lua.NewTranslator("doc",
		filter.WithUntrustedInput(),
		filter.WithAllowedFields(allowedFields...),
	)
	require.NoError(tb, err)

	return tr
}

// FuzzTranslateKeepsLiteralsClosed is the injection oracle for the Lua backend.
//
// The output is a Lua boolean expression that something later evaluates, so a
// caller value that closed its own quote does not corrupt a query — it becomes
// executable code. `" or true or "` as a value would turn a filter into a
// tautology, and the same hole reaches further in a language with function
// calls in it.
func FuzzTranslateKeepsLiteralsClosed(f *testing.F) {
	for _, seed := range filterSeeds {
		f.Add(seed)
	}

	parser, err := filter.NewParser(filter.WithParserNoCache())
	require.NoError(f, err)

	f.Fuzz(func(t *testing.T, expression string) {
		node, err := parser.Parse(t.Context(), expression)
		if err != nil {
			return
		}

		script, err := newTranslator(t).Translate(node)
		if err != nil {
			require.Empty(t, script, "a rejected expression must not also produce Lua")
			return
		}

		require.True(t, endsOutsideLiteral(script),
			"a value escaped its string literal — the expression ends mid-literal:\n%s", script)
	})
}

// FuzzTranslateRejectsFieldsOutsideTheAllowlist states the allowlist property
// against the AST rather than against the generated Lua: if the expression
// names a field the allowlist does not cover, translation must fail.
//
// Reading it back out of the output would mean telling a field reference from a
// Lua identifier, and every keyword the oracle forgot would be a false alarm
// whose fix is to widen the oracle — which is how a check stops checking.
func FuzzTranslateRejectsFieldsOutsideTheAllowlist(f *testing.F) {
	for _, seed := range filterSeeds {
		f.Add(seed)
	}
	f.Add(`secret == "x"`)
	f.Add(`password.contains("a")`)
	f.Add(`name == "x" && internal_flag == true`)

	parser, err := filter.NewParser(filter.WithParserNoCache())
	require.NoError(f, err)

	f.Fuzz(func(t *testing.T, expression string) {
		node, err := parser.Parse(t.Context(), expression)
		if err != nil {
			return
		}

		_, err = newTranslator(t).Translate(node)

		for _, field := range identifiers(node) {
			if !slices.Contains(allowedFields, field) {
				// Any rejection will do. Demanding ErrFieldNotAllowed
				// specifically would fire on expressions that are invalid for a
				// second reason too — `"0" && A` is refused as a bare literal
				// before the allow-list is ever consulted — and the property
				// here is that translation does not succeed.
				require.Error(t, err,
					"%q names %q, which the allowlist does not cover", expression, field)
				return
			}
		}
	})
}

// endsOutsideLiteral walks generated Lua and reports whether it ends outside a
// string literal. Values are double-quoted with backslash escapes, so a
// backslash consumes the byte after it and only an unescaped quote toggles the
// state.
func endsOutsideLiteral(script string) bool {
	inLiteral := false
	for i := 0; i < len(script); i++ {
		switch {
		case inLiteral && script[i] == '\\':
			i++
		case script[i] == '"':
			inLiteral = !inLiteral
		}
	}
	return !inLiteral
}

// identifiers collects every field name an expression references, at any depth.
func identifiers(node filter.Node) []string {
	if node == nil {
		return nil
	}

	var out []string
	if ident, ok := node.(*filter.IdentNode); ok {
		out = append(out, ident.Name)
	}
	for child := range node.Children() {
		out = append(out, identifiers(child)...)
	}
	return out
}
