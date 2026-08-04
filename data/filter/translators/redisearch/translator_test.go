// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redisearch

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
func mustTranslator(tb testing.TB, schema map[string]FieldType, opts ...filter.TranslatorOption) *Translator {
	tb.Helper()
	tr, err := NewTranslator(schema, opts...)
	require.NoError(tb, err)
	return tr
}

// testSchema provides a schema for testing with common field types.
var testSchema = map[string]FieldType{
	"id":          FieldTypeTag,
	"name":        FieldTypeText,
	"description": FieldTypeText,
	"status":      FieldTypeNumeric,
	"priority":    FieldTypeNumeric,
	"age":         FieldTypeNumeric,
	"schedule":    FieldTypeTag,
	"active":      FieldTypeTag,
	"role":        FieldTypeTag,
	"lastRunAt":   FieldTypeNumeric,
	"nextRunAt":   FieldTypeNumeric,
	"failures":    FieldTypeNumeric,
	"skipNextRun": FieldTypeTag,
	"unmanaged":   FieldTypeTag,
	"oneShot":     FieldTypeTag,
	"createdAt":   FieldTypeNumeric,
	"updatedAt":   FieldTypeNumeric,
}

func TestTranslator_NumericComparisons(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want string
	}{
		{
			name: "equal",
			expr: `status == 1`,
			want: `@status:[1 1]`,
		},
		{
			name: "not equal",
			expr: `status != 0`,
			want: `-@status:[0 0]`,
		},
		{
			name: "greater than",
			expr: `age > 18`,
			want: `@age:[(18 +inf]`,
		},
		{
			name: "greater than or equal",
			expr: `age >= 18`,
			want: `@age:[18 +inf]`,
		},
		{
			name: "less than",
			expr: `priority < 10`,
			want: `@priority:[-inf (10]`,
		},
		{
			name: "less than or equal",
			expr: `priority <= 10`,
			want: `@priority:[-inf 10]`,
		},
	}

	trans := mustTranslator(t, testSchema)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := testhelpers.MustParseFilter(t, tt.expr)
			got, err := trans.Translate(node)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestTranslator_TagComparisons(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want string
	}{
		{
			name: "equal string",
			expr: `id == "task-1"`,
			want: `@id:{task\-1}`,
		},
		{
			name: "not equal string",
			expr: `schedule != "daily"`,
			want: `-@schedule:{daily}`,
		},
		{
			name: "equal bool true",
			expr: `active == true`,
			want: `@active:{true}`,
		},
		{
			name: "equal bool false",
			expr: `active == false`,
			want: `@active:{false}`,
		},
	}

	trans := mustTranslator(t, testSchema)

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
			name: "and",
			expr: `status == 1 && priority >= 3`,
			want: `(@status:[1 1] @priority:[3 +inf])`,
		},
		{
			name: "or",
			expr: `status == 1 || status == 4`,
			want: `(@status:[1 1])|(@status:[4 4])`,
		},
		{
			name: "not simple ident",
			expr: `!active`,
			want: `-@active:{true}`,
		},
		{
			name: "not complex expression",
			expr: `!(status == 1)`,
			want: `-(@status:[1 1])`,
		},
	}

	trans := mustTranslator(t, testSchema)

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
			name: "string list",
			expr: `role in ["admin", "editor"]`,
			want: `@role:{admin|editor}`,
		},
		{
			name: "single value",
			expr: `id in ["abc"]`,
			want: `@id:{abc}`,
		},
	}

	trans := mustTranslator(t, testSchema)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := testhelpers.MustParseFilter(t, tt.expr)
			got, err := trans.Translate(node)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestTranslator_StringFunctions(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want string
	}{
		{
			name: "contains",
			expr: `name.contains("sub")`,
			want: `@name:*sub*`,
		},
		{
			name: "startsWith",
			expr: `name.startsWith("pre")`,
			want: `@name:pre*`,
		},
	}

	trans := mustTranslator(t, testSchema)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := testhelpers.MustParseFilter(t, tt.expr)
			got, err := trans.Translate(node)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestTranslator_UnsupportedOperations(t *testing.T) {
	tests := []struct {
		name string
		expr string
	}{
		{name: "endsWith", expr: `name.endsWith("x")`},
		{name: "matches", expr: `name.matches("^A.*")`},
		{name: "size", expr: `name.size() == 3`},
	}

	trans := mustTranslator(t, testSchema)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := testhelpers.MustParseFilter(t, tt.expr)
			_, err := trans.Translate(node)
			require.ErrorIs(t, err, filter.ErrUnsupportedOperation)
		})
	}
}

func TestTranslator_NilNode(t *testing.T) {
	trans := mustTranslator(t, testSchema)

	got, err := trans.Translate(nil)
	require.NoError(t, err)
	require.Equal(t, "*", got)
}

func TestTranslator_ComplexExpressions(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want string
	}{
		{
			name: "and with or",
			expr: `status == 1 && (role in ["admin", "editor"])`,
			want: `(@status:[1 1] @role:{admin|editor})`,
		},
		{
			name: "triple and",
			expr: `status == 1 && priority >= 3 && failures == 0`,
			want: `((@status:[1 1] @priority:[3 +inf]) @failures:[0 0])`,
		},
	}

	trans := mustTranslator(t, testSchema)

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
	trans := mustTranslator(t, testSchema, filter.WithAllowedFields("status", "priority"))

	t.Run("allowed field", func(t *testing.T) {
		node := testhelpers.MustParseFilter(t, `status == 1`)
		_, err := trans.Translate(node)
		require.NoError(t, err)
	})

	t.Run("disallowed field", func(t *testing.T) {
		node := testhelpers.MustParseFilter(t, `name == "test"`)
		_, err := trans.Translate(node)
		require.ErrorIs(t, err, filter.ErrFieldNotAllowed)
	})
}

func TestTranslator_WithFieldMapping(t *testing.T) {
	trans := mustTranslator(t,
		map[string]FieldType{
			"mapped_status": FieldTypeNumeric,
		},
		filter.WithFieldMapping(map[string]string{
			"status": "mapped_status",
		}),
	)

	node := testhelpers.MustParseFilter(t, `status == 1`)
	got, err := trans.Translate(node)
	require.NoError(t, err)
	require.Equal(t, `@mapped_status:[1 1]`, got)
}

func TestTranslator_WithMaxDepth(t *testing.T) {
	trans := mustTranslator(t, testSchema, filter.WithMaxDepth(2))

	t.Run("within depth", func(t *testing.T) {
		node := testhelpers.MustParseFilter(t, `status == 1 && priority >= 3`)
		_, err := trans.Translate(node)
		require.NoError(t, err)
	})

	t.Run("exceeds depth", func(t *testing.T) {
		node := testhelpers.MustParseFilter(t, `(status == 1 && priority >= 3) && (failures == 0 && age > 10)`)
		_, err := trans.Translate(node)
		require.ErrorIs(t, err, filter.ErrMaxDepthExceeded)
	})
}

func TestTranslator_TagEscaping(t *testing.T) {
	trans := mustTranslator(t, map[string]FieldType{
		"tag": FieldTypeTag,
	})

	tests := []struct {
		name string
		expr string
		want string
	}{
		{
			name: "special chars",
			expr: `tag == "hello:world"`,
			want: `@tag:{hello\:world}`,
		},
		{
			name: "dash",
			expr: `tag == "my-tag"`,
			want: `@tag:{my\-tag}`,
		},
		{
			name: "at sign",
			expr: `tag == "user@test"`,
			want: `@tag:{user\@test}`,
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
	trans := mustTranslator(t, testSchema)
	var _ filter.Visitor = trans
}

// TestNewTranslator_UntrustedInput_RequiresAllowlist locks the contract
// that misconfiguration is rejected at construction, not at the first
// Translate call.
func TestNewTranslator_UntrustedInput_RequiresAllowlist(t *testing.T) {
	t.Parallel()

	_, err := NewTranslator(testSchema, filter.WithUntrustedInput())
	require.ErrorIs(t, err, filter.ErrAllowlistRequired)

	_, err = NewTranslator(testSchema,
		filter.WithUntrustedInput(),
		filter.WithAllowedFields("status"),
	)
	require.NoError(t, err)
}

// TestNewTranslator_UntrustedInput_EmptyAllowlist guards the
// misconfiguration path where WithAllowedFields was called with no
// arguments (or with an empty slice from a config loader). The check
// has to happen at construction so the bug surfaces during boot, not
// on the first untrusted query.
func TestNewTranslator_UntrustedInput_EmptyAllowlist(t *testing.T) {
	t.Parallel()

	_, err := NewTranslator(testSchema,
		filter.WithUntrustedInput(),
		filter.WithAllowedFields(),
	)
	require.ErrorIs(t, err, filter.ErrAllowlistRequired)

	var empty []string
	_, err = NewTranslator(testSchema,
		filter.WithUntrustedInput(),
		filter.WithAllowedFields(empty...),
	)
	require.ErrorIs(t, err, filter.ErrAllowlistRequired)
}

// TestTranslator_BareIdentifier pins the boolean-test rendering of a bare
// identifier. translateNot has always handled the negated form; without
// its counterpart a bare identifier reached the server as a free-text
// term, matching whatever the TEXT fields happened to contain rather
// than failing.
func TestTranslator_BareIdentifier(t *testing.T) {
	schema := map[string]FieldType{
		"active":   FieldTypeTag,
		"verified": FieldTypeTag,
		"age":      FieldTypeNumeric,
	}

	tests := []struct {
		name string
		expr string
		want string
	}{
		{"at the root", `active`, `@active:{true}`},
		{"negated at the root", `!active`, `-@active:{true}`},
		{"as a conjunct", `active && verified`, `(@active:{true} @verified:{true})`},
		{"mixed with a comparison", `active && age > 18`, `(@active:{true} @age:[(18 +inf])`},
		{"as a disjunct", `active || verified`, `(@active:{true})|(@verified:{true})`},
	}

	trans := mustTranslator(t, schema)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := testhelpers.MustParseFilter(t, tt.expr)
			got, err := trans.Translate(node)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

// TestTranslator_InFollowsFieldSchema pins the membership syntax per
// schema type. The tag form is not universal: `@role:{2|3}` against a
// NUMERIC field matched nothing at all — silently, which is worse than
// failing — and against a TEXT field it is a syntax error.
func TestTranslator_InFollowsFieldSchema(t *testing.T) {
	schema := map[string]FieldType{
		"status": FieldTypeTag,
		"role":   FieldTypeNumeric,
		"price":  FieldTypeNumeric,
		"name":   FieldTypeText,
	}

	tests := []struct {
		name string
		expr string
		want string
	}{
		{
			"tag field keeps the tag set",
			`status in ["active", "pending"]`,
			`@status:{active|pending}`,
		},
		{
			"numeric field becomes a union of exact ranges",
			`role in [2, 3]`,
			`(@role:[2 2]|@role:[3 3])`,
		},
		{
			"numeric field with floats",
			`price in [10.5, 20]`,
			`(@price:[10.5 10.5]|@price:[20 20])`,
		},
		{
			"text field becomes a term union",
			`name in ["Alice", "Bob"]`,
			`@name:(Alice|Bob)`,
		},
		{
			"single numeric element",
			`role in [1]`,
			`(@role:[1 1])`,
		},
	}

	trans := mustTranslator(t, schema)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := testhelpers.MustParseFilter(t, tt.expr)
			got, err := trans.Translate(node)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

// TestTranslator_BareIdentifierRespectsPolicy guards that the new path
// goes through the allow-list and field mapping like every other field
// reference.
func TestTranslator_BareIdentifierRespectsPolicy(t *testing.T) {
	schema := map[string]FieldType{"is_active": FieldTypeTag}

	t.Run("mapping applies", func(t *testing.T) {
		trans := mustTranslator(t, schema,
			filter.WithFieldMapping(map[string]string{"isActive": "is_active"}))
		node := testhelpers.MustParseFilter(t, `isActive`)

		got, err := trans.Translate(node)
		require.NoError(t, err)
		require.Equal(t, `@is_active:{true}`, got)
	})

	t.Run("allow-list applies", func(t *testing.T) {
		trans := mustTranslator(t, schema, filter.WithAllowedFields("name"))
		node := testhelpers.MustParseFilter(t, `active`)

		_, err := trans.Translate(node)
		require.ErrorIs(t, err, filter.ErrFieldNotAllowed)
	})
}
