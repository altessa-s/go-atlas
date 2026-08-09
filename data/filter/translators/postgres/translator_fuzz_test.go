// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package postgres_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/filter"
	"github.com/altessa-s/go-atlas/data/filter/translators/postgres"
)

// allowedFields is the allowlist every target translates against. Untrusted
// input requires one, and pinning it here means a field name that reached the
// output but is not in this list is a finding rather than a configuration.
var allowedFields = []string{"name", "age", "active", "created_at", "tags", "score"}

// filterSeeds are expressions worth starting from: ordinary predicates, and the
// shapes that would break out of a literal if quoting were wrong.
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
	`name == "\x00"`,
	`unknown_field == "x"`,
	``,
	`((((`,
}

// endsOutsideLiteral walks generated PostgreSQL and reports whether the text
// ends outside a string literal.
//
// This is the whole question for inline rendering. A caller-supplied value that
// escaped its quoting leaves the scanner inside a literal at the end of the
// clause (an odd quote), or turns the tail into SQL that was meant to be data.
// The dialect writes E-strings, where a backslash escapes the next byte, so the
// scanner has to model that rather than counting quotes.
func endsOutsideLiteral(sql string) bool {
	inLiteral := false
	for i := 0; i < len(sql); i++ {
		switch {
		case inLiteral && sql[i] == '\\':
			i++ // Skip the escaped byte, whatever it is.
		case sql[i] == '\'':
			inLiteral = !inLiteral
		}
	}
	return !inLiteral
}

// newTranslator builds the untrusted-input translator the targets share.
func newTranslator(tb testing.TB) *postgres.Translator {
	tb.Helper()

	tr, err := postgres.NewTranslator(
		filter.WithUntrustedInput(),
		filter.WithAllowedFields(allowedFields...),
	)
	require.NoError(tb, err)

	return tr
}

// FuzzTranslateInlineKeepsLiteralsClosed is the SQL-injection oracle for the
// inline form.
//
// TranslateInline puts caller-controlled values into the SQL text itself, so
// its safety rests entirely on the dialect's quoting — its own doc comment says
// as much. The parser already has a fuzz target; this one covers the step after
// it, where a value that survived parsing becomes SQL. Feeding whole
// expressions rather than the dialect's quoter directly is deliberate: it is
// the composition of parse and render that has to hold, and a value can reach
// the renderer through a comparison, an IN list, or a string predicate, each
// with its own path.
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

// FuzzTranslateBindsEveryPlaceholder pins the parameterized form's own
// invariant: the clause and the argument slice must agree.
//
// PostgreSQL placeholders are numbered, so a walker that collected an argument
// without emitting its marker (or the reverse) produces a statement the driver
// rejects, or worse, one that binds the wrong value to the wrong position. The
// count is checked rather than the text because the numbering is what carries
// the association.
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

		for i := range args {
			marker := "$" + strconv.Itoa(i+1)
			require.Contains(t, sql, marker,
				"argument %d has no placeholder in the clause:\n%s\nargs: %#v", i+1, sql, args)
		}
		require.NotContains(t, sql, "$"+strconv.Itoa(len(args)+1),
			"the clause references a placeholder with no argument behind it:\n%s\nargs: %#v", sql, args)
	})
}

// FuzzTranslateHonorsTheAllowlist checks the field allowlist end to end: no
// column outside it may appear in the generated clause.
//
// The allowlist is the boundary between "a client may filter on this" and "a
// client may make the database scan an unindexed column", and it is enforced
// during the walk rather than at parse time — a field can enter the AST through
// a comparison, a function call target, or an IN list, and each reaches the
// check by a different route.
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

		// Quoted identifiers are the only place a column name appears; every
		// value went to a placeholder.
		for _, ident := range quotedIdents(sql) {
			require.Contains(t, allowedFields, ident,
				"a column outside the allowlist reached the clause: %q\n%s", ident, sql)
		}
	})
}

// quotedIdents extracts the double-quoted identifiers from a clause.
func quotedIdents(sql string) []string {
	var out []string
	for rest := sql; ; {
		open := strings.IndexByte(rest, '"')
		if open < 0 {
			return out
		}
		rest = rest[open+1:]
		closeAt := strings.IndexByte(rest, '"')
		if closeAt < 0 {
			return out
		}
		out = append(out, rest[:closeAt])
		rest = rest[closeAt+1:]
	}
}
