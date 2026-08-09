// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package meili_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/filter"
	"github.com/altessa-s/go-atlas/data/filter/translators/meili"
)

var allowedFields = []string{"name", "age", "active", "created_at", "tags", "score"}

var filterSeeds = []string{
	`name == "John"`,
	`age > 18 && active == true`,
	`name.contains("oh")`,
	`name in ["a", "b"]`,
	`name == "\""`,
	`name == "\\"`,
	`name == "\" OR name = \"admin"`,
	`name == "a AND age > 0"`,
	`name.startsWith("\"")`,
	`unknown_field == "x"`,
	``,
}

func newTranslator(tb testing.TB) *meili.Translator {
	tb.Helper()

	tr, err := meili.NewTranslator(
		filter.WithUntrustedInput(),
		filter.WithAllowedFields(allowedFields...),
	)
	require.NoError(tb, err)

	return tr
}

// FuzzTranslateKeepsLiteralsClosed is the injection oracle for Meilisearch.
//
// Meilisearch takes a filter as a *string* — there is no placeholder to bind a
// value to, so every caller value is rendered into the expression and the
// dialect's quoting is the only separation there is. A value that closed its
// own quote would continue as filter syntax: `" OR name = "admin` reads as a
// disjunction the caller never wrote, which is how a search endpoint starts
// returning documents outside its tenant.
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

		clause, err := newTranslator(t).Translate(node)
		if err != nil {
			require.Empty(t, clause, "a rejected expression must not also produce a filter")
			return
		}

		require.True(t, endsOutsideLiteral(clause),
			"a value escaped its string literal — the filter ends mid-literal:\n%s", clause)
	})
}

// FuzzTranslateRejectsFieldsOutsideTheAllowlist states the allowlist property
// directly instead of looking for field names in the output.
//
// Reading the generated filter back would mean re-implementing Meilisearch's
// grammar to tell an attribute from an operator — and every operator the oracle
// forgets (CONTAINS, STARTS WITH) is a false alarm whose fix is to widen the
// oracle, which is how a check quietly stops checking. Walking the AST instead
// says exactly what matters: if the expression names a field the allowlist does
// not, translation must fail.
func FuzzTranslateRejectsFieldsOutsideTheAllowlist(f *testing.F) {
	for _, seed := range filterSeeds {
		f.Add(seed)
	}
	f.Add(`secret == "x"`)
	f.Add(`password.contains("a")`)
	f.Add(`name == "x" && internal_flag == true`)
	f.Add(`name in ["a"] && secret in ["b"]`)

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

// endsOutsideLiteral walks a generated Meilisearch filter and reports whether
// it ends outside a string literal. Values are double-quoted with backslash
// escapes, so a backslash consumes the byte after it and only an unescaped
// quote toggles the state.
func endsOutsideLiteral(clause string) bool {
	inLiteral := false
	for i := 0; i < len(clause); i++ {
		switch {
		case inLiteral && clause[i] == '\\':
			i++
		case clause[i] == '"':
			inLiteral = !inLiteral
		}
	}
	return !inLiteral
}
