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

	trans := NewTranslator(testSchema)

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

	trans := NewTranslator(testSchema)

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

	trans := NewTranslator(testSchema)

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

	trans := NewTranslator(testSchema)

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

	trans := NewTranslator(testSchema)

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

	trans := NewTranslator(testSchema)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := testhelpers.MustParseFilter(t, tt.expr)
			_, err := trans.Translate(node)
			require.ErrorIs(t, err, filter.ErrUnsupportedOperation)
		})
	}
}

func TestTranslator_NilNode(t *testing.T) {
	trans := NewTranslator(testSchema)

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

	trans := NewTranslator(testSchema)

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
	trans := NewTranslator(testSchema, filter.WithAllowedFields("status", "priority"))

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
	trans := NewTranslator(
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
	trans := NewTranslator(testSchema, filter.WithMaxDepth(2))

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
	trans := NewTranslator(map[string]FieldType{
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
	trans := NewTranslator(testSchema)
	var _ filter.Visitor = trans
}

func TestNewTranslator_DefaultConfig(t *testing.T) {
	trans := NewTranslator(testSchema)
	require.NotNil(t, trans)
	require.NotNil(t, trans.config)
	require.Equal(t, filter.DefaultMaxDepth, trans.config.MaxDepth())
}
