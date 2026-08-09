// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mariadb_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/filter"
	"github.com/altessa-s/go-atlas/data/filter/translators/mariadb"
)

// allowedFields is the allowlist every target translates against.
var allowedFields = []string{"name", "age", "active", "created_at", "tags", "score"}

// filterSeeds are ordinary predicates plus the shapes that would break out of a
// literal if the dialect's quoting were wrong.
var filterSeeds = []string{
	`name == "John"`,
	`age > 18 && active == true`,
	`name.contains("oh")`,
	`name.matches("^a.*z$")`,
	`tags.size() > 2`,
	`name in ["a", "b"]`,
	`name == "'"`,
	`name == "''"`,
	`name == "\\"`,
	`name == "\\'"`,
	`name == "'; DROP TABLE users; --"`,
	`name == "' OR 1=1 --"`,
	`name.startsWith("'")`,
	`name.endsWith("\\")`,
	`unknown_field == "x"`,
	``,
}

func newTranslator(tb testing.TB) *mariadb.Translator {
	tb.Helper()

	tr, err := mariadb.NewTranslator(
		filter.WithUntrustedInput(),
		filter.WithAllowedFields(allowedFields...),
	)
	require.NoError(tb, err)

	return tr
}

// FuzzTranslateInlineKeepsLiteralsClosed is the injection oracle for the inline
// form, where caller values become SQL text and the dialect's quoting is the
// only thing between them and the statement.
//
// The scanner below models this dialect specifically: MariaDB doubles a quote to escape it (”), and backslash-escapes the control bytes. The shared walker
// is covered by the PostgreSQL target; what is dialect-specific — and therefore
// only checkable here — is the escaping.
func FuzzTranslateInlineKeepsLiteralsClosed(f *testing.F) {
	for _, seed := range filterSeeds {
		f.Add(seed)
	}

	parser, err := filter.NewParser(filter.WithParserNoCache())
	require.NoError(f, err)

	f.Fuzz(func(t *testing.T, expression string) {
		node, err := parser.Parse(t.Context(), expression)
		if err != nil {
			return // Rejected at parse; the translator never sees it.
		}

		sql, err := newTranslator(t).TranslateInline(node)
		if err != nil {
			require.Empty(t, sql, "a rejected expression must not also produce SQL")
			return
		}

		require.True(t, endsOutsideLiteral(sql),
			"a value escaped its string literal — the clause ends mid-literal:\n%s", sql)
	})
}

// FuzzTranslateBindsEveryPlaceholder pins the parameterized form: the clause
// carries exactly one positional marker per collected argument.
//
// This dialect's marker is positional (?), so the association is by order and a
// count mismatch is a statement that binds the wrong value to the wrong slot —
// or one the driver refuses outright. Markers inside a literal do not count,
// which is why they are tallied by the same scanner.
func FuzzTranslateBindsEveryPlaceholder(f *testing.F) {
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

		sql, args, err := newTranslator(t).Translate(node)
		if err != nil {
			require.Empty(t, sql, "a rejected expression must not also produce SQL")
			require.Empty(t, args, "a rejected expression must not also produce arguments")
			return
		}

		require.Equal(t, len(args), countPlaceholders(sql),
			"placeholders and arguments disagree:\n%s\nargs: %#v", sql, args)
	})
}

// FuzzTranslateHonorsTheAllowlist checks the field allowlist end to end: a
// column outside it must never reach the clause, whichever route the field took
// through the AST — a comparison, a call target, or an IN list.
func FuzzTranslateHonorsTheAllowlist(f *testing.F) {
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

		sql, _, err := newTranslator(t).Translate(node)
		if err != nil {
			return
		}

		for _, ident := range backquotedIdents(sql) {
			require.Contains(t, allowedFields, ident,
				"a column outside the allowlist reached the clause: %q\n%s", ident, sql)
		}
	})
}

// backquotedIdents extracts the backtick-quoted identifiers from a clause.
// Every value went to a placeholder, so these are the only column references.
func backquotedIdents(sql string) []string {
	parts := strings.Split(sql, "`")

	var out []string
	// Split on a paired delimiter puts the quoted spans at the odd indices.
	for i := 1; i < len(parts); i += 2 {
		out = append(out, parts[i])
	}
	return out
}

// endsOutsideLiteral walks generated MariaDB SQL and reports whether the text
// ends outside a string literal.
//
// MariaDB has two escape mechanisms at once, and the scanner has to model both:
// a quote is escaped by doubling it (”), while a backslash escapes the byte
// that follows. Handling only one of them would make the scanner disagree with
// the dialect on exactly the inputs this target exists to explore.
func endsOutsideLiteral(sql string) bool {
	inLiteral := false
	for i := 0; i < len(sql); i++ {
		switch {
		case inLiteral && sql[i] == '\\':
			i++ // Backslash escape: skip the escaped byte.
		case inLiteral && sql[i] == '\'' && i+1 < len(sql) && sql[i+1] == '\'':
			i++ // Doubled quote: an escaped ', not the end of the literal.
		case sql[i] == '\'':
			inLiteral = !inLiteral
		}
	}
	return !inLiteral
}

// countPlaceholders counts the bind markers outside string literals. A literal
// '?' is data, not a marker.
func countPlaceholders(sql string) int {
	n, inLiteral := 0, false
	for i := 0; i < len(sql); i++ {
		switch {
		case inLiteral && sql[i] == '\\':
			i++
		case inLiteral && sql[i] == '\'' && i+1 < len(sql) && sql[i+1] == '\'':
			i++
		case sql[i] == '\'':
			inLiteral = !inLiteral
		case !inLiteral && sql[i] == '?':
			n++
		}
	}
	return n
}
