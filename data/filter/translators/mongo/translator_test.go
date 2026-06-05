// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas/data/filter"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

func bsonToJSON(m bson.M) string {
	b, _ := json.Marshal(m)
	return string(b)
}

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
		name     string
		expr     string
		wantJSON string
	}{
		{
			name:     "equal string",
			expr:     `name == "John"`,
			wantJSON: `{"name":"John"}`,
		},
		{
			name:     "equal int",
			expr:     `age == 25`,
			wantJSON: `{"age":25}`,
		},
		{
			name:     "equal bool",
			expr:     `active == true`,
			wantJSON: `{"active":true}`,
		},
		{
			name:     "not equal",
			expr:     `status != "deleted"`,
			wantJSON: `{"status":{"$ne":"deleted"}}`,
		},
		{
			name:     "greater than",
			expr:     `age > 18`,
			wantJSON: `{"age":{"$gt":18}}`,
		},
		{
			name:     "greater than or equal",
			expr:     `age >= 18`,
			wantJSON: `{"age":{"$gte":18}}`,
		},
		{
			name:     "less than",
			expr:     `age < 65`,
			wantJSON: `{"age":{"$lt":65}}`,
		},
		{
			name:     "less than or equal",
			expr:     `age <= 65`,
			wantJSON: `{"age":{"$lte":65}}`,
		},
	}

	trans := mustTranslator(t)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := testhelpers.MustParseFilter(t, tt.expr)
			result, err := trans.Translate(node)
			require.NoError(t, err)
			got := bsonToJSON(result)
			require.Equal(t, tt.wantJSON, got)
		})
	}
}

func TestTranslator_LogicalOperators(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		wantJSON string
	}{
		{
			name:     "and",
			expr:     `name == "John" && age >= 18`,
			wantJSON: `{"$and":[{"name":"John"},{"age":{"$gte":18}}]}`,
		},
		{
			name:     "or",
			expr:     `status == "active" || status == "pending"`,
			wantJSON: `{"$or":[{"status":"active"},{"status":"pending"}]}`,
		},
		{
			name:     "not simple",
			expr:     `!active`,
			wantJSON: `{"active":{"$ne":true}}`,
		},
	}

	trans := mustTranslator(t)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := testhelpers.MustParseFilter(t, tt.expr)
			result, err := trans.Translate(node)
			require.NoError(t, err)
			got := bsonToJSON(result)
			require.Equal(t, tt.wantJSON, got)
		})
	}
}

func TestTranslator_NestedFields(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		wantJSON string
	}{
		{
			name:     "two levels",
			expr:     `address.city == "NYC"`,
			wantJSON: `{"address.city":"NYC"}`,
		},
		{
			name:     "three levels",
			expr:     `user.profile.name == "John"`,
			wantJSON: `{"user.profile.name":"John"}`,
		},
	}

	trans := mustTranslator(t)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := testhelpers.MustParseFilter(t, tt.expr)
			result, err := trans.Translate(node)
			require.NoError(t, err)
			got := bsonToJSON(result)
			require.Equal(t, tt.wantJSON, got)
		})
	}
}

func TestTranslator_InOperator(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		wantJSON string
	}{
		{
			name:     "string list",
			expr:     `status in ["active", "pending"]`,
			wantJSON: `{"status":{"$in":["active","pending"]}}`,
		},
		{
			name:     "int list",
			expr:     `priority in [1, 2, 3]`,
			wantJSON: `{"priority":{"$in":[1,2,3]}}`,
		},
	}

	trans := mustTranslator(t)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := testhelpers.MustParseFilter(t, tt.expr)
			result, err := trans.Translate(node)
			require.NoError(t, err)
			got := bsonToJSON(result)
			require.Equal(t, tt.wantJSON, got)
		})
	}
}

func TestTranslator_StringFunctions(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		wantJSON string
	}{
		{
			name:     "contains",
			expr:     `name.contains("oh")`,
			wantJSON: `{"name":{"$regex":"oh"}}`,
		},
		{
			name:     "startsWith",
			expr:     `name.startsWith("J")`,
			wantJSON: `{"name":{"$regex":"^J"}}`,
		},
		{
			name:     "endsWith",
			expr:     `name.endsWith("n")`,
			wantJSON: `{"name":{"$regex":"n$"}}`,
		},
		{
			name:     "matches",
			expr:     `name.matches("^[A-Z].*")`,
			wantJSON: `{"name":{"$regex":"^[A-Z].*"}}`,
		},
		{
			name:     "contains special chars",
			expr:     `name.contains("a.b")`,
			wantJSON: `{"name":{"$regex":"a\\.b"}}`,
		},
	}

	trans := mustTranslator(t)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := testhelpers.MustParseFilter(t, tt.expr)
			result, err := trans.Translate(node)
			require.NoError(t, err)
			got := bsonToJSON(result)
			require.Equal(t, tt.wantJSON, got)
		})
	}
}

func TestTranslator_MatchesRegexValidation(t *testing.T) {
	trans := mustTranslator(t)

	t.Run("valid regex", func(t *testing.T) {
		node := testhelpers.MustParseFilter(t, `name.matches("^[A-Z][a-z]+$")`)
		_, err := trans.Translate(node)
		require.NoError(t, err)
	})

	t.Run("invalid regex", func(t *testing.T) {
		node := testhelpers.MustParseFilter(t, `name.matches("[invalid")`)
		_, err := trans.Translate(node)
		require.ErrorIs(t, err, filter.ErrInvalidRegex)
	})

	t.Run("too long regex", func(t *testing.T) {
		// Build a regex that exceeds default maxRegexLength (1024).
		// Tests the method-bound regexPassthrough directly because the
		// CEL parser may reject the synthetic 1025-byte literal before
		// it reaches the translator.
		trans := mustTranslator(t)
		_, err := trans.regexPassthrough(string(make([]byte, 1025)))
		require.ErrorIs(t, err, filter.ErrInvalidRegex)
	})

	t.Run("WithMaxRegexLength is honored", func(t *testing.T) {
		// Regression guard: WithMaxRegexLength was previously ignored by
		// the mongo translator (regexPassthrough hardcoded
		// DefaultMaxRegexLength). The configured cap must now actually
		// apply.
		trans := mustTranslator(t, filter.WithMaxRegexLength(10))
		_, err := trans.regexPassthrough(string(make([]byte, 64)))
		require.ErrorIs(t, err, filter.ErrInvalidRegex,
			"WithMaxRegexLength(10) must reject 64-byte pattern")
	})
}

func TestTranslator_HasFunction(t *testing.T) {
	trans := mustTranslator(t)

	node := testhelpers.MustParseFilter(t, `has(user.email)`)
	result, err := trans.Translate(node)
	require.NoError(t, err)
	got := bsonToJSON(result)
	require.Equal(t, `{"user.email":{"$exists":true}}`, got)
}

func TestTranslator_SizeFunction(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want string
	}{
		{
			name: "size equal",
			expr: `tags.size() == 3`,
			want: `{"tags":{"$size":3}}`,
		},
		{
			name: "size not equal",
			expr: `tags.size() != 0`,
			want: `{"tags":{"$not":{"$size":0}}}`,
		},
	}

	trans := mustTranslator(t)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := testhelpers.MustParseFilter(t, tt.expr)
			result, err := trans.Translate(node)
			require.NoError(t, err)
			got := bsonToJSON(result)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestTranslator_ComplexExpressions(t *testing.T) {
	tests := []struct {
		name string
		expr string
	}{
		{
			name: "compound and",
			expr: `name == "John" && age >= 18 && active == true`,
		},
		{
			name: "in with and",
			expr: `status in ["active", "pending"] && age >= 18`,
		},
		{
			name: "string func with and",
			expr: `name.startsWith("J") && active == true`,
		},
		{
			name: "nested field with comparison",
			expr: `address.city == "NYC" && address.zip == "10001"`,
		},
	}

	trans := mustTranslator(t)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := testhelpers.MustParseFilter(t, tt.expr)
			result, err := trans.Translate(node)
			require.NoError(t, err)
			require.NotNil(t, result)
		})
	}
}

func TestTranslator_WithAllowedFields(t *testing.T) {
	trans := mustTranslator(t,
		filter.WithAllowedFields("name", "age"))

	t.Run("allowed field", func(t *testing.T) {
		node := testhelpers.MustParseFilter(t, `name == "John"`)
		_, err := trans.Translate(node)
		require.NoError(t, err)
	})

	t.Run("disallowed field", func(t *testing.T) {
		node := testhelpers.MustParseFilter(t, `email == "test@example.com"`)
		_, err := trans.Translate(node)
		require.ErrorIs(t, err, filter.ErrFieldNotAllowed)
	})
}

func TestTranslator_WithFieldMapping(t *testing.T) {
	trans := mustTranslator(t,
		filter.WithFieldMapping(map[string]string{
			"userName":  "user_name",
			"createdAt": "created_at",
		}))

	tests := []struct {
		name     string
		expr     string
		wantJSON string
	}{
		{
			name:     "mapped field",
			expr:     `userName == "john"`,
			wantJSON: `{"user_name":"john"}`,
		},
		{
			name:     "unmapped field",
			expr:     `email == "test@example.com"`,
			wantJSON: `{"email":"test@example.com"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := testhelpers.MustParseFilter(t, tt.expr)
			result, err := trans.Translate(node)
			require.NoError(t, err)
			got := bsonToJSON(result)
			require.Equal(t, tt.wantJSON, got)
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

func TestTranslator_TimestampComparison(t *testing.T) {
	trans := mustTranslator(t)

	node := testhelpers.MustParseFilter(t, `created_at >= timestamp("2024-01-01T00:00:00Z")`)
	result, err := trans.Translate(node)
	require.NoError(t, err)
	require.NotNil(t, result)

	createdAt, ok := result["created_at"].(bson.M)
	require.True(t, ok, "unexpected result structure: %+v", result)
	_, hasGte := createdAt["$gte"]
	require.True(t, hasGte, "expected $gte operator for timestamp comparison")
}

func TestTranslator_WithFieldTypes(t *testing.T) {
	trans := mustTranslator(t,
		filter.WithFieldTypes(map[string]filter.FieldKind{
			"status":    filter.FieldKindInt,
			"active":    filter.FieldKindBool,
			"name":      filter.FieldKindString,
			"price":     filter.FieldKindFloat,
			"createdAt": filter.FieldKindTimestamp,
		}))

	t.Run("match", func(t *testing.T) {
		tests := []struct{ expr string }{
			{`status == 1`},
			{`status in [1, 2, 3]`},
			{`active == true`},
			{`name == "Alice"`},
			{`price >= 100`},
			{`price >= 100.0`},
			{`createdAt >= timestamp("2024-01-01T00:00:00Z")`},
			{`status == null`},
			{`other == "anything"`},
		}
		for _, tt := range tests {
			t.Run(tt.expr, func(t *testing.T) {
				node := testhelpers.MustParseFilter(t, tt.expr)
				_, err := trans.Translate(node)
				require.NoError(t, err)
			})
		}
	})

	t.Run("mismatch", func(t *testing.T) {
		tests := []struct{ expr string }{
			{`status == "qwer"`},
			{`status == 1.5`},
			{`active == 1`},
			{`name == 42`},
			{`status in [1, "qwer", 3]`},
			{`createdAt == "2024-01-01"`},
		}
		for _, tt := range tests {
			t.Run(tt.expr, func(t *testing.T) {
				node := testhelpers.MustParseFilter(t, tt.expr)
				_, err := trans.Translate(node)
				require.ErrorIs(t, err, filter.ErrFieldTypeMismatch)
			})
		}
	})
}

func TestTranslator_NullValue(t *testing.T) {
	trans := mustTranslator(t)

	node := testhelpers.MustParseFilter(t, `deleted_at == null`)
	result, err := trans.Translate(node)
	require.NoError(t, err)
	require.Equal(t, `{"deleted_at":null}`, bsonToJSON(result))
}

// TestNewTranslator_UntrustedInput_RequiresAllowlist locks the contract
// that misconfiguration is rejected at construction, not at the first
// Translate call.
func TestNewTranslator_UntrustedInput_RequiresAllowlist(t *testing.T) {
	t.Parallel()

	_, err := NewTranslator(filter.WithUntrustedInput())
	require.ErrorIs(t, err, filter.ErrAllowlistRequired)

	_, err = NewTranslator(
		filter.WithUntrustedInput(),
		filter.WithAllowedFields("name"),
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

	_, err := NewTranslator(
		filter.WithUntrustedInput(),
		filter.WithAllowedFields(),
	)
	require.ErrorIs(t, err, filter.ErrAllowlistRequired)

	var empty []string
	_, err = NewTranslator(
		filter.WithUntrustedInput(),
		filter.WithAllowedFields(empty...),
	)
	require.ErrorIs(t, err, filter.ErrAllowlistRequired)
}

func TestTranslatorContext_ApplyFieldMapping(t *testing.T) {
	ctx, err := filter.NewTranslatorContext(filter.WithFieldMapping(map[string]string{
		"userName": "user_name",
	}))
	require.NoError(t, err)

	t.Run("mapped field", func(t *testing.T) {
		require.Equal(t, "user_name", ctx.ApplyFieldMapping("userName"))
	})

	t.Run("unmapped field", func(t *testing.T) {
		require.Equal(t, "email", ctx.ApplyFieldMapping("email"))
	})

	t.Run("nil mapping", func(t *testing.T) {
		bare, err := filter.NewTranslatorContext()
		require.NoError(t, err)
		require.Equal(t, "any", bare.ApplyFieldMapping("any"))
	})
}

func TestTranslatorContext_IsFieldAllowed(t *testing.T) {
	t.Run("no allowlist", func(t *testing.T) {
		ctx, err := filter.NewTranslatorContext()
		require.NoError(t, err)
		require.True(t, ctx.IsFieldAllowed("any_field"), "IsFieldAllowed should return true when no allowlist is set")
	})

	t.Run("with allowlist", func(t *testing.T) {
		ctx, err := filter.NewTranslatorContext(filter.WithAllowedFields("name", "age"))
		require.NoError(t, err)
		require.True(t, ctx.IsFieldAllowed("name"))
		require.False(t, ctx.IsFieldAllowed("email"))
	})
}

func TestTranslator_VisitorInterface(t *testing.T) {
	trans := mustTranslator(t)

	// Verify it implements filter.Visitor
	var _ filter.Visitor = trans
}

func TestTranslator_BuiltinPresets(t *testing.T) {
	tests := []struct {
		name     string
		funcs    map[string]filter.CustomFunction
		expr     string
		wantJSON string
	}{
		{
			name:     "between expands to and-tree",
			funcs:    filter.BetweenFilter(),
			expr:     `between(age, 18, 65)`,
			wantJSON: `{"$and":[{"age":{"$gte":18}},{"age":{"$lte":65}}]}`,
		},
		{
			name:     "notDeleted expands to null equality",
			funcs:    filter.SoftDeleteFilters(),
			expr:     `notDeleted()`,
			wantJSON: `{"deletedAt":null}`,
		},
		{
			name:     "onlyDeleted expands to $ne null",
			funcs:    filter.SoftDeleteFilters(),
			expr:     `onlyDeleted()`,
			wantJSON: `{"deletedAt":{"$ne":null}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser, err := filter.NewParser(
				filter.WithParserNoCache(),
				filter.WithCustomFunctions(tt.funcs),
			)
			require.NoError(t, err)

			node, err := parser.Parse(t.Context(), tt.expr)
			require.NoError(t, err)

			got, err := mustTranslator(t).Translate(node)
			require.NoError(t, err)
			require.JSONEq(t, tt.wantJSON, bsonToJSON(got))
		})
	}
}

func TestTranslator_CustomFunction_CompareField(t *testing.T) {
	parser, err := filter.NewParser(
		filter.WithParserNoCache(),
		filter.WithCustomFunctions(map[string]filter.CustomFunction{
			"createdAfter": filter.CompareField("createdAt", filter.OpGT),
		}),
	)
	require.NoError(t, err)

	t.Run("plain target field", func(t *testing.T) {
		node, err := parser.Parse(t.Context(), `createdAfter("2024-01-01")`)
		require.NoError(t, err)

		got, err := mustTranslator(t).Translate(node)
		require.NoError(t, err)
		require.JSONEq(t, `{"createdAt":{"$gt":"2024-01-01"}}`, bsonToJSON(got))
	})

	t.Run("with field mapping to db column", func(t *testing.T) {
		node, err := parser.Parse(t.Context(), `createdAfter("2024-01-01")`)
		require.NoError(t, err)

		trans := mustTranslator(t,
			filter.WithFieldMapping(map[string]string{
				"createdAt": "created_at",
			}))
		got, err := trans.Translate(node)
		require.NoError(t, err)
		require.JSONEq(t, `{"created_at":{"$gt":"2024-01-01"}}`, bsonToJSON(got))
	})
}

func TestTranslator_EnumValues(t *testing.T) {
	trans := mustTranslator(t,
		filter.WithEnumValues(map[string][]int64{
			"role": {1, 2, 3, 4, 6, 7},
		}))

	t.Run("in range", func(t *testing.T) {
		tests := []struct {
			name     string
			expr     string
			wantJSON string
		}{
			{
				name:     "equal",
				expr:     `role == 7`,
				wantJSON: `{"role":7}`,
			},
			{
				name:     "in list",
				expr:     `role in [1, 2, 3]`,
				wantJSON: `{"role":{"$in":[1,2,3]}}`,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				node := testhelpers.MustParseFilter(t, tt.expr)
				result, err := trans.Translate(node)
				require.NoError(t, err)
				require.JSONEq(t, tt.wantJSON, bsonToJSON(result))
			})
		}
	})

	t.Run("out of range", func(t *testing.T) {
		exprs := []string{
			`role == 10`,
			`role != 9`,
			`role >= 8`,
			`role in [1, 5, 7]`,
		}

		for _, expr := range exprs {
			t.Run(expr, func(t *testing.T) {
				node := testhelpers.MustParseFilter(t, expr)
				_, err := trans.Translate(node)
				require.ErrorIs(t, err, filter.ErrEnumValueNotAllowed)
			})
		}
	})
}
