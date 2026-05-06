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

	trans := NewTranslator()

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

	trans := NewTranslator()

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

	trans := NewTranslator()

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

	trans := NewTranslator()

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
	trans := NewTranslator()
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

	trans := NewTranslator()

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
	trans := NewTranslator()

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
	trans := NewTranslator()

	node := testhelpers.MustParseFilter(t, `has(user.email)`)
	got, err := trans.Translate(node)
	require.NoError(t, err)
	require.Equal(t, `user.email EXISTS`, got)
}

func TestTranslator_SizeFunction_Unsupported(t *testing.T) {
	trans := NewTranslator()

	t.Run("size equality", func(t *testing.T) {
		node := testhelpers.MustParseFilter(t, `tags.size() == 3`)
		_, err := trans.Translate(node)
		require.ErrorIs(t, err, filter.ErrUnsupportedOperation)
	})

	t.Run("bare size call", func(t *testing.T) {
		node := testhelpers.MustParseFilter(t, `size(tags)`)
		_, err := trans.Translate(node)
		require.ErrorIs(t, err, filter.ErrUnsupportedOperation)
	})
}

func TestTranslator_NullValue(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want string
	}{
		{"is null", `deleted_at == null`, `deleted_at IS NULL`},
		{"is not null", `deleted_at != null`, `deleted_at IS NOT NULL`},
	}

	trans := NewTranslator()

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
	trans := NewTranslator()
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

	trans := NewTranslator()

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
	trans := NewTranslator(filter.WithAllowedFields("name", "age"))

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

func TestTranslator_UntrustedInputRequiresAllowlist(t *testing.T) {
	trans := NewTranslator(filter.WithUntrustedInput())

	node := testhelpers.MustParseFilter(t, `name == "John"`)
	_, err := trans.Translate(node)
	require.ErrorIs(t, err, filter.ErrAllowlistRequired)
}

func TestTranslator_WithFieldMapping(t *testing.T) {
	trans := NewTranslator(filter.WithFieldMapping(map[string]string{
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
	trans := NewTranslator(filter.WithMaxDepth(2))

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
	trans := NewTranslator(
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

func TestNewTranslator_DefaultConfig(t *testing.T) {
	trans := NewTranslator()
	require.NotNil(t, trans)
	require.NotNil(t, trans.config)
	require.Equal(t, filter.DefaultMaxDepth, trans.config.MaxDepth())
}

func TestTranslator_VisitorInterface(t *testing.T) {
	trans := NewTranslator()
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
