// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package meili

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/filter"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

// mustTranslator builds a translator and fails the test on any
// construction error. Keeps the success-path tests free of
// error-wiring noise; tests that exercise construction failures call
// [NewTranslator] directly.
func mustTranslator(tb testing.TB, opts ...filter.TranslatorOption) *Translator {
	tb.Helper()
	tr, err := NewTranslator(opts...)
	require.NoError(tb, err)
	return tr
}

func TestTranslator_BasicComparisons(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want string
	}{
		{"equal string", `name == "John"`, `name = "John"`},
		{"equal int", `age == 25`, `age = 25`},
		{"equal bool", `active == true`, `active = true`},
		{"not equal string", `status != "deleted"`, `status != "deleted"`},
		{"not equal int", `status != 0`, `status != 0`},
		{"greater than", `age > 18`, `age > 18`},
		{"greater than or equal", `age >= 18`, `age >= 18`},
		{"less than", `age < 65`, `age < 65`},
		{"less than or equal", `age <= 65`, `age <= 65`},
	}

	trans := mustTranslator(t)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := testhelpers.MustParseFilter(t, tt.expr)
			got, err := trans.Translate(node)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestTranslator_LogicalOperators(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want string
	}{
		{
			"and",
			`name == "John" && age >= 18`,
			`(name = "John") AND (age >= 18)`,
		},
		{
			"or",
			`status == "active" || status == "pending"`,
			`(status = "active") OR (status = "pending")`,
		},
		{
			"not on bare ident",
			`!active`,
			`NOT (active = true)`,
		},
		{
			"not on expression",
			`!(age >= 18)`,
			`NOT (age >= 18)`,
		},
		{
			"chained and",
			`a == 1 && b == 2 && c == 3`,
			`((a = 1) AND (b = 2)) AND (c = 3)`,
		},
	}

	trans := mustTranslator(t)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := testhelpers.MustParseFilter(t, tt.expr)
			got, err := trans.Translate(node)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestTranslator_NestedFields(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want string
	}{
		{"two levels", `address.city == "NYC"`, `address.city = "NYC"`},
		{"three levels", `user.profile.name == "John"`, `user.profile.name = "John"`},
	}

	trans := mustTranslator(t)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := testhelpers.MustParseFilter(t, tt.expr)
			got, err := trans.Translate(node)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestTranslator_InOperator(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want string
	}{
		{
			"string list",
			`status in ["active", "pending"]`,
			`status IN ["active", "pending"]`,
		},
		{
			"int list",
			`priority in [1, 2, 3]`,
			`priority IN [1, 2, 3]`,
		},
		{
			"single element",
			`type in [1]`,
			`type IN [1]`,
		},
	}

	trans := mustTranslator(t)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := testhelpers.MustParseFilter(t, tt.expr)
			got, err := trans.Translate(node)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestTranslator_InOperator_ValueInField_Rejected(t *testing.T) {
	// `'value' in field` is rejected for parity with the MongoDB translator.
	trans := mustTranslator(t)
	node := testhelpers.MustParseFilter(t, `"units" in dictionaryCodes`)
	_, err := trans.Translate(node)
	require.ErrorIs(t, err, filter.ErrInvalidExpression)
}

func TestTranslator_StringFunctions(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want string
	}{
		{"contains", `name.contains("oh")`, `name CONTAINS "oh"`},
		{"startsWith", `name.startsWith("J")`, `name STARTS WITH "J"`},
	}

	trans := mustTranslator(t)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := testhelpers.MustParseFilter(t, tt.expr)
			got, err := trans.Translate(node)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestTranslator_UnsupportedStringFunctions(t *testing.T) {
	trans := mustTranslator(t)

	t.Run("endsWith", func(t *testing.T) {
		node := testhelpers.MustParseFilter(t, `name.endsWith("x")`)
		_, err := trans.Translate(node)
		require.ErrorIs(t, err, filter.ErrUnsupportedOperation)
	})

	t.Run("matches", func(t *testing.T) {
		node := testhelpers.MustParseFilter(t, `name.matches("^[A-Z].*$")`)
		_, err := trans.Translate(node)
		require.ErrorIs(t, err, filter.ErrUnsupportedOperation)
	})
}

func TestTranslator_HasFunction(t *testing.T) {
	// CEL's `has()` macro requires a field-selection expression (dotted path),
	// not a bare identifier — that's a parser-level rule.
	trans := mustTranslator(t)

	node := testhelpers.MustParseFilter(t, `has(user.email)`)
	got, err := trans.Translate(node)
	require.NoError(t, err)
	require.Equal(t, `user.email EXISTS`, got)
}

func TestTranslator_SizeFunction_Unsupported(t *testing.T) {
	trans := mustTranslator(t)

	t.Run("size equality lhs", func(t *testing.T) {
		node := testhelpers.MustParseFilter(t, `tags.size() == 3`)
		_, err := trans.Translate(node)
		require.ErrorIs(t, err, filter.ErrUnsupportedOperation)
	})

	t.Run("size equality rhs", func(t *testing.T) {
		node := testhelpers.MustParseFilter(t, `3 == tags.size()`)
		_, err := trans.Translate(node)
		require.ErrorIs(t, err, filter.ErrUnsupportedOperation)
	})

	t.Run("size ordering rhs", func(t *testing.T) {
		node := testhelpers.MustParseFilter(t, `5 < tags.size()`)
		_, err := trans.Translate(node)
		require.ErrorIs(t, err, filter.ErrUnsupportedOperation)
	})

	t.Run("bare size call", func(t *testing.T) {
		node := testhelpers.MustParseFilter(t, `size(tags)`)
		_, err := trans.Translate(node)
		require.ErrorIs(t, err, filter.ErrUnsupportedOperation)
	})
}

func TestTranslator_Timestamp(t *testing.T) {
	// timestamp() literals are emitted as Unix seconds — Meilisearch filters
	// numeric attributes only, and sub-second precision is dropped.
	trans := mustTranslator(t)

	tests := []struct {
		name string
		expr string
		want string
	}{
		{
			"equality",
			`createdAt == timestamp("2024-01-02T03:04:05Z")`,
			`createdAt = 1704164645`,
		},
		{
			"greater than",
			`createdAt > timestamp("2024-01-02T03:04:05Z")`,
			`createdAt > 1704164645`,
		},
		{
			"sub-second precision is dropped",
			`createdAt == timestamp("2024-01-02T03:04:05.789Z")`,
			`createdAt = 1704164645`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := testhelpers.MustParseFilter(t, tt.expr)
			got, err := trans.Translate(node)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

// TestTranslator_NullValue pins both halves of the null question.
//
// Meilisearch splits what CEL's null conflates: an attribute can be
// absent from a document, or present and null. IS NULL alone answers
// only the second, so a filter built from it would miss every document
// that omits the attribute — and its negation would match all of them,
// which is exactly how a soft-delete filter leaks deleted documents.
func TestTranslator_NullValue(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want string
	}{
		{"is null", `deleted_at == null`, `(deleted_at IS NULL OR deleted_at NOT EXISTS)`},
		{"is not null", `deleted_at != null`, `(deleted_at EXISTS AND deleted_at IS NOT NULL)`},
	}

	trans := mustTranslator(t)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := testhelpers.MustParseFilter(t, tt.expr)
			got, err := trans.Translate(node)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestTranslator_NullValue_OrderingUnsupported(t *testing.T) {
	trans := mustTranslator(t)
	node := testhelpers.MustParseFilter(t, `deleted_at > null`)
	_, err := trans.Translate(node)
	require.ErrorIs(t, err, filter.ErrUnsupportedOperation)
}

func TestTranslator_StringEscaping(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want string
	}{
		{
			"backslash",
			`name == "a\\b"`,
			`name = "a\\b"`,
		},
		{
			"quote",
			`name == "a\"b"`,
			`name = "a\"b"`,
		},
		{
			"both",
			`name == "a\\\"b"`,
			`name = "a\\\"b"`,
		},
	}

	trans := mustTranslator(t)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := testhelpers.MustParseFilter(t, tt.expr)
			got, err := trans.Translate(node)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestTranslator_WithAllowedFields(t *testing.T) {
	trans := mustTranslator(t,
		filter.WithAllowedFields("name", "age"))

	t.Run("allowed", func(t *testing.T) {
		node := testhelpers.MustParseFilter(t, `name == "John"`)
		_, err := trans.Translate(node)
		require.NoError(t, err)
	})

	t.Run("disallowed", func(t *testing.T) {
		node := testhelpers.MustParseFilter(t, `email == "test@example.com"`)
		_, err := trans.Translate(node)
		require.ErrorIs(t, err, filter.ErrFieldNotAllowed)
	})
}

func TestNewTranslator_UntrustedInputRequiresAllowlist(t *testing.T) {
	_, err := NewTranslator(filter.WithUntrustedInput())
	require.ErrorIs(t, err, filter.ErrAllowlistRequired)
}

func TestTranslator_WithFieldMapping(t *testing.T) {
	trans := mustTranslator(t,
		filter.WithFieldMapping(map[string]string{
			"organizationIds": "organization_id",
			"dictionaryCodes": "dictionary_code",
		}))

	tests := []struct {
		name string
		expr string
		want string
	}{
		{
			"mapped field equality",
			`organizationIds == "abc"`,
			`organization_id = "abc"`,
		},
		{
			"mapped field in list",
			`dictionaryCodes in ["currencies", "units"]`,
			`dictionary_code IN ["currencies", "units"]`,
		},
		{
			"unmapped field passes through",
			`status == 2`,
			`status = 2`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := testhelpers.MustParseFilter(t, tt.expr)
			got, err := trans.Translate(node)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestTranslator_WithMaxDepth(t *testing.T) {
	trans := mustTranslator(t,
		filter.WithMaxDepth(2))

	t.Run("within depth", func(t *testing.T) {
		node := testhelpers.MustParseFilter(t, `name == "John" && age >= 18`)
		_, err := trans.Translate(node)
		require.NoError(t, err)
	})

	t.Run("exceeds depth", func(t *testing.T) {
		node := testhelpers.MustParseFilter(t, `(name == "John" && age >= 18) && (status == "active" && type == "user")`)
		_, err := trans.Translate(node)
		require.ErrorIs(t, err, filter.ErrMaxDepthExceeded)
	})
}

func TestTranslator_RealProtoCases(t *testing.T) {
	// Cases drawn from
	// proto/services/grpc/dictionaries/v1/search/dictionaries_search_service.proto.
	trans := mustTranslator(t,

		filter.WithAllowedFields("status", "type", "visibility", "organizationIds", "dictionaryCodes"),
		filter.WithFieldMapping(map[string]string{
			"organizationIds": "organization_id",
			"dictionaryCodes": "dictionary_code",
		}),
	)

	tests := []struct {
		name string
		expr string
		want string
	}{
		{
			"single status",
			`status == 2`,
			`status = 2`,
		},
		{
			"type in list and visibility eq",
			`type in [1, 2] && visibility == 1`,
			`(type IN [1, 2]) AND (visibility = 1)`,
		},
		{
			"organizationIds membership via list shape",
			`organizationIds in ["org-uuid"]`,
			`organization_id IN ["org-uuid"]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := testhelpers.MustParseFilter(t, tt.expr)
			got, err := trans.Translate(node)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestTranslator_VisitorInterface(t *testing.T) {
	trans := mustTranslator(t)
	var _ filter.Visitor = trans
}

func TestQuoteString(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", `""`},
		{"hello", `"hello"`},
		{`a"b`, `"a\"b"`},
		{`a\b`, `"a\\b"`},
		{`a\"b`, `"a\\\"b"`},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			require.Equal(t, tt.want, quoteString(tt.in))
		})
	}
}

// TestTranslator_BareIdentifierAtRoot pins the boolean-test rendering of
// a bare identifier used as a whole expression. acceptString has always
// handled it inside && and ||; at the root Translate visited the node
// directly and emitted the bare attribute name, which Meilisearch
// rejects as a missing operator.
func TestTranslator_BareIdentifierAtRoot(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want string
	}{
		{"at the root", `active`, `active = true`},
		{"negated at the root", `!active`, `NOT (active = true)`},
		{"as a conjunct", `active && verified`, `(active = true) AND (verified = true)`},
	}

	trans := mustTranslator(t)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := testhelpers.MustParseFilter(t, tt.expr)
			got, err := trans.Translate(node)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

// TestTranslator_BareIdentifierRespectsPolicy guards that the root path
// goes through the allow-list and field mapping like every other field
// reference.
func TestTranslator_BareIdentifierRespectsPolicy(t *testing.T) {
	t.Run("mapping applies", func(t *testing.T) {
		trans := mustTranslator(t, filter.WithFieldMapping(map[string]string{"isActive": "is_active"}))
		node := testhelpers.MustParseFilter(t, `isActive`)

		got, err := trans.Translate(node)
		require.NoError(t, err)
		require.Equal(t, `is_active = true`, got)
	})

	t.Run("allow-list applies", func(t *testing.T) {
		trans := mustTranslator(t, filter.WithAllowedFields("name"))
		node := testhelpers.MustParseFilter(t, `active`)

		_, err := trans.Translate(node)
		require.ErrorIs(t, err, filter.ErrFieldNotAllowed)
	})
}

// TestTranslator_NullComposesUnderNegation checks that the parenthesized
// null forms survive being wrapped, which is what makes them safe to
// compose rather than a string that only works standalone.
func TestTranslator_NullComposesUnderNegation(t *testing.T) {
	trans := mustTranslator(t)
	node := testhelpers.MustParseFilter(t, `!(deletedAt == null) && status == "x"`)

	got, err := trans.Translate(node)
	require.NoError(t, err)
	require.Equal(t,
		`(NOT ((deletedAt IS NULL OR deletedAt NOT EXISTS))) AND (status = "x")`,
		got)
}
