// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filter_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/filter"

	"github.com/altessa-s/go-atlas/data/filter/translators/clickhouse"
	"github.com/altessa-s/go-atlas/data/filter/translators/lua"
	"github.com/altessa-s/go-atlas/data/filter/translators/mariadb"
	"github.com/altessa-s/go-atlas/data/filter/translators/meili"
	filtermongo "github.com/altessa-s/go-atlas/data/filter/translators/mongo"
	"github.com/altessa-s/go-atlas/data/filter/translators/postgres"
	"github.com/altessa-s/go-atlas/data/filter/translators/redisearch"
)

// TestTranslators_BareLiteralIsNotAPredicate is a cross-backend regression for
// a query-injection hole a fuzz target found in the SQL translator.
//
// Every translator decided "did this node produce a clause?" by type-asserting
// the walk's result to the backend's clause type. For the text backends that
// type is `string` — and a CEL expression that is just a string literal makes
// VisitLiteral return exactly that, a Go string. The assertion could not tell
// the two apart, so the literal was returned as the clause and emitted into the
// query verbatim: unquoted, unbound, and past the field allow-list.
//
// `parser.Parse(ctx, userInput)` followed by `translator.Translate(node)` is the
// documented shape for untrusted filters, so the payload below is a complete
// filter expression a client could send. It lives here rather than in each
// backend's package because the invariant is one statement about all of them,
// and a per-backend copy is what let the hole persist in six places at once.
func TestTranslators_BareLiteralIsNotAPredicate(t *testing.T) {
	t.Parallel()

	parser, err := filter.NewParser(filter.WithParserNoCache())
	require.NoError(t, err)

	backends := translatorBackends()

	// Each of these parses cleanly into a single literal node — the parser has
	// no reason to object, which is what made the translator the only line of
	// defense.
	expressions := map[string]string{
		"sql injection payload": `"'; DROP TABLE users; --"`,
		"tautology":             `"1=1 OR 1=1"`,
		"bare quote":            `"'"`,
		"plain string":          `"plain"`,
		"empty string":          `""`,
		"integer":               `123`,
		"boolean":               `true`,
	}

	for backendName, translate := range backends {
		for exprName, expression := range expressions {
			t.Run(backendName+"/"+exprName, func(t *testing.T) {
				t.Parallel()

				node, parseErr := parser.Parse(t.Context(), expression)
				require.NoError(t, parseErr, "the parser accepts this; the translator is what must refuse it")

				require.ErrorIs(t, translate(t, node), filter.ErrInvalidExpression,
					"a bare literal was accepted as a filter clause")
			})
		}
	}
}

// TestTranslators_CallWithoutTargetIsRejected is a regression for a remote
// crash a fuzz target found: `contains()` — a call the parser accepts, with no
// target — made the Meilisearch, Lua and RediSearch translators dereference a
// nil Node. A filter expression a client controls could therefore panic the
// process, which is a denial of service in any handler that does not recover.
//
// The SQL and MongoDB translators already reported it as a malformed
// expression; this pins that every backend does.
func TestTranslators_CallWithoutTargetIsRejected(t *testing.T) {
	t.Parallel()

	parser, err := filter.NewParser(filter.WithParserNoCache())
	require.NoError(t, err)

	for backendName, translate := range translatorBackends() {
		for _, expression := range []string{"contains()", "startsWith()", "endsWith()", "size()"} {
			t.Run(backendName+"/"+expression, func(t *testing.T) {
				t.Parallel()

				node, parseErr := parser.Parse(t.Context(), expression)
				if parseErr != nil {
					t.Skipf("the parser rejects %q on its own: %v", expression, parseErr)
				}

				// The assertion is that this returns at all: a panic here is
				// the bug, and the error value only has to be non-nil.
				require.Error(t, translate(t, node),
					"%q must be rejected, not translated", expression)
			})
		}
	}
}

// translatorBackends is every filter backend, reduced to "translate this node,
// report the error". Sharing one table across the cross-backend regressions is
// what keeps a newly added backend from being covered by none of them.
func translatorBackends() map[string]func(t *testing.T, node filter.Node) error {
	opts := []filter.TranslatorOption{
		filter.WithUntrustedInput(),
		filter.WithAllowedFields("name"),
	}

	return map[string]func(t *testing.T, node filter.Node) error{
		"postgres": func(t *testing.T, node filter.Node) error {
			tr, err := postgres.NewTranslator(opts...)
			require.NoError(t, err)
			_, _, translateErr := tr.Translate(node)
			return translateErr
		},
		"mariadb": func(t *testing.T, node filter.Node) error {
			tr, err := mariadb.NewTranslator(opts...)
			require.NoError(t, err)
			_, _, translateErr := tr.Translate(node)
			return translateErr
		},
		"clickhouse": func(t *testing.T, node filter.Node) error {
			tr, err := clickhouse.NewTranslator(opts...)
			require.NoError(t, err)
			_, _, translateErr := tr.Translate(node)
			return translateErr
		},
		"mongo": func(t *testing.T, node filter.Node) error {
			tr, err := filtermongo.NewTranslator(opts...)
			require.NoError(t, err)
			_, translateErr := tr.Translate(node)
			return translateErr
		},
		"meili": func(t *testing.T, node filter.Node) error {
			tr, err := meili.NewTranslator(opts...)
			require.NoError(t, err)
			_, translateErr := tr.Translate(node)
			return translateErr
		},
		"lua": func(t *testing.T, node filter.Node) error {
			tr, err := lua.NewTranslator("doc", opts...)
			require.NoError(t, err)
			_, translateErr := tr.Translate(node)
			return translateErr
		},
		"redisearch": func(t *testing.T, node filter.Node) error {
			tr, err := redisearch.NewTranslator(
				map[string]redisearch.FieldType{"name": redisearch.FieldTypeText}, opts...)
			require.NoError(t, err)
			_, translateErr := tr.Translate(node)
			return translateErr
		},
	}
}
