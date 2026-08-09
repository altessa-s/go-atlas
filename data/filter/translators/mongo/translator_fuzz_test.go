// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas/data/filter"
	filtermongo "github.com/altessa-s/go-atlas/data/filter/translators/mongo"
)

// allowedFields is the allowlist every target translates against.
var allowedFields = []string{"name", "age", "active", "created_at", "tags", "score"}

// knownOperators is the closed set of MongoDB operators this translator emits,
// transcribed from its source rather than from what MongoDB accepts. That is
// the point: anything else with a leading `$` in a key came from caller data,
// and an operator injection is how a filter becomes a `$where` or a `$function`
// the server evaluates. Widening this list to make a failure go away would
// throw the target away — check where the operator came from first.
var knownOperators = []string{
	"$and", "$or", "$not", "$nor",
	"$ne", "$gt", "$gte", "$lt", "$lte",
	"$in", "$regex", "$exists", "$size", "$expr", "$ifNull",
}

var filterSeeds = []string{
	`name == "John"`,
	`age > 18 && active == true`,
	`name.contains("oh")`,
	`name.matches("^a.*z$")`,
	`tags.size() > 2`,
	`name in ["a", "b"]`,
	`!active`,
	`name == "$ne"`,
	`name == "$where"`,
	`name == "{\"$gt\": \"\"}"`,
	`name.matches("$where")`,
	`unknown_field == "x"`,
	``,
	`((((`,
}

func newTranslator(tb testing.TB) *filtermongo.Translator {
	tb.Helper()

	tr, err := filtermongo.NewTranslator(
		filter.WithUntrustedInput(),
		filter.WithAllowedFields(allowedFields...),
	)
	require.NoError(tb, err)

	return tr
}

// FuzzTranslateEmitsNoUnknownOperator is the operator-injection oracle.
//
// A MongoDB filter is a document, not a string, so there is no quoting to break
// out of — the equivalent hole is a caller-controlled value landing in *key*
// position, where the server reads it as an operator. The check walks the whole
// document and demands that every `$`-prefixed key is one this translator is
// supposed to produce; a value that reached a key would show up as an operator
// nobody wrote.
func FuzzTranslateEmitsNoUnknownOperator(f *testing.F) {
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

		doc, err := newTranslator(t).Translate(node)
		if err != nil {
			return
		}

		walkKeys(doc, func(key string) {
			if !strings.HasPrefix(key, "$") {
				return
			}
			require.True(t, slices.Contains(knownOperators, key),
				"an operator this translator never emits reached the query: %q\n%v", key, doc)
		})
	})
}

// FuzzTranslateHonorsTheAllowlist pins the field allowlist end to end.
//
// A field can enter the AST through a comparison, a function call target, or an
// IN list, and each reaches the allow-list check by a different route. The
// document's non-operator keys are exactly its field references, so walking
// them is a complete statement about what the query may touch.
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

		doc, err := newTranslator(t).Translate(node)
		if err != nil {
			return
		}

		walkKeys(doc, func(key string) {
			if strings.HasPrefix(key, "$") {
				return
			}
			require.True(t, slices.Contains(allowedFields, key),
				"a field outside the allowlist reached the query: %q\n%v", key, doc)
		})
	})
}

// walkKeys visits every map key in a decoded filter document, at any depth.
func walkKeys(value any, visit func(key string)) {
	switch v := value.(type) {
	case bson.M:
		for key, child := range v {
			visit(key)
			walkKeys(child, visit)
		}
	case bson.D:
		for _, elem := range v {
			visit(elem.Key)
			walkKeys(elem.Value, visit)
		}
	case []any:
		for _, child := range v {
			walkKeys(child, visit)
		}
	case bson.A:
		for _, child := range v {
			walkKeys(child, visit)
		}
	}
}
