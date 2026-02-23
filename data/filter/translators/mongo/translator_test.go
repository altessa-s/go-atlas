// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"encoding/json"
	"errors"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas/data/filter"
)

func mustParse(t *testing.T, expr string) filter.Node {
	t.Helper()
	p, err := filter.NewParser(filter.WithParserNoCache())
	if err != nil {
		t.Fatalf("NewParser() error = %v", err)
	}
	node, err := p.Parse(expr)
	if err != nil {
		t.Fatalf("Parse(%q) error = %v", expr, err)
	}
	return node
}

func bsonToJSON(m bson.M) string {
	b, _ := json.Marshal(m)
	return string(b)
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

	trans := NewTranslator()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := mustParse(t, tt.expr)
			result, err := trans.Translate(node)
			if err != nil {
				t.Fatalf("Translate() error = %v", err)
			}
			got := bsonToJSON(result)
			if got != tt.wantJSON {
				t.Errorf("Translate() = %s, want %s", got, tt.wantJSON)
			}
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

	trans := NewTranslator()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := mustParse(t, tt.expr)
			result, err := trans.Translate(node)
			if err != nil {
				t.Fatalf("Translate() error = %v", err)
			}
			got := bsonToJSON(result)
			if got != tt.wantJSON {
				t.Errorf("Translate() = %s, want %s", got, tt.wantJSON)
			}
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

	trans := NewTranslator()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := mustParse(t, tt.expr)
			result, err := trans.Translate(node)
			if err != nil {
				t.Fatalf("Translate() error = %v", err)
			}
			got := bsonToJSON(result)
			if got != tt.wantJSON {
				t.Errorf("Translate() = %s, want %s", got, tt.wantJSON)
			}
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

	trans := NewTranslator()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := mustParse(t, tt.expr)
			result, err := trans.Translate(node)
			if err != nil {
				t.Fatalf("Translate() error = %v", err)
			}
			got := bsonToJSON(result)
			if got != tt.wantJSON {
				t.Errorf("Translate() = %s, want %s", got, tt.wantJSON)
			}
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

	trans := NewTranslator()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := mustParse(t, tt.expr)
			result, err := trans.Translate(node)
			if err != nil {
				t.Fatalf("Translate() error = %v", err)
			}
			got := bsonToJSON(result)
			if got != tt.wantJSON {
				t.Errorf("Translate() = %s, want %s", got, tt.wantJSON)
			}
		})
	}
}

func TestTranslator_MatchesRegexValidation(t *testing.T) {
	trans := NewTranslator()

	t.Run("valid regex", func(t *testing.T) {
		node := mustParse(t, `name.matches("^[A-Z][a-z]+$")`)
		_, err := trans.Translate(node)
		if err != nil {
			t.Fatalf("Translate() error = %v for valid regex", err)
		}
	})

	t.Run("invalid regex", func(t *testing.T) {
		node := mustParse(t, `name.matches("[invalid")`)
		_, err := trans.Translate(node)
		if !errors.Is(err, filter.ErrInvalidRegex) {
			t.Errorf("Translate() error = %v, want %v", err, filter.ErrInvalidRegex)
		}
	})

	t.Run("too long regex", func(t *testing.T) {
		// Build a regex that exceeds maxRegexLength (1024)
		longPattern := `name.matches("` + string(make([]byte, 1025)) + `")`
		// We can't use mustParse for this since CEL parser may reject it.
		// Instead test the regexPassthrough function directly.
		_, err := regexPassthrough(string(make([]byte, 1025)))
		if !errors.Is(err, filter.ErrInvalidRegex) {
			t.Errorf("regexPassthrough() error = %v, want %v", err, filter.ErrInvalidRegex)
		}
		_ = longPattern // avoid unused
	})
}

func TestTranslator_HasFunction(t *testing.T) {
	trans := NewTranslator()

	node := mustParse(t, `has(user.email)`)
	result, err := trans.Translate(node)
	if err != nil {
		t.Fatalf("Translate() error = %v", err)
	}
	got := bsonToJSON(result)
	want := `{"user.email":{"$exists":true}}`
	if got != want {
		t.Errorf("Translate() = %s, want %s", got, want)
	}
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

	trans := NewTranslator()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := mustParse(t, tt.expr)
			result, err := trans.Translate(node)
			if err != nil {
				t.Fatalf("Translate() error = %v", err)
			}
			got := bsonToJSON(result)
			if got != tt.want {
				t.Errorf("Translate() = %s, want %s", got, tt.want)
			}
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

	trans := NewTranslator()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := mustParse(t, tt.expr)
			result, err := trans.Translate(node)
			if err != nil {
				t.Fatalf("Translate() error = %v", err)
			}
			if result == nil {
				t.Error("Translate() returned nil")
			}
		})
	}
}

func TestTranslator_WithAllowedFields(t *testing.T) {
	trans := NewTranslator(filter.WithAllowedFields("name", "age"))

	t.Run("allowed field", func(t *testing.T) {
		node := mustParse(t, `name == "John"`)
		_, err := trans.Translate(node)
		if err != nil {
			t.Errorf("Translate() error = %v for allowed field", err)
		}
	})

	t.Run("disallowed field", func(t *testing.T) {
		node := mustParse(t, `email == "test@example.com"`)
		_, err := trans.Translate(node)
		if !errors.Is(err, filter.ErrFieldNotAllowed) {
			t.Errorf("Translate() error = %v, want %v", err, filter.ErrFieldNotAllowed)
		}
	})
}

func TestTranslator_WithFieldMapping(t *testing.T) {
	trans := NewTranslator(filter.WithFieldMapping(map[string]string{
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
			node := mustParse(t, tt.expr)
			result, err := trans.Translate(node)
			if err != nil {
				t.Fatalf("Translate() error = %v", err)
			}
			got := bsonToJSON(result)
			if got != tt.wantJSON {
				t.Errorf("Translate() = %s, want %s", got, tt.wantJSON)
			}
		})
	}
}

func TestTranslator_WithMaxDepth(t *testing.T) {
	trans := NewTranslator(filter.WithMaxDepth(2))

	t.Run("within depth", func(t *testing.T) {
		node := mustParse(t, `name == "John" && age >= 18`)
		_, err := trans.Translate(node)
		if err != nil {
			t.Errorf("Translate() error = %v for valid depth", err)
		}
	})

	t.Run("exceeds depth", func(t *testing.T) {
		node := mustParse(t, `(name == "John" && age >= 18) && (status == "active" && type == "user")`)
		_, err := trans.Translate(node)
		if !errors.Is(err, filter.ErrMaxDepthExceeded) {
			t.Errorf("Translate() error = %v, want %v", err, filter.ErrMaxDepthExceeded)
		}
	})
}

func TestTranslator_TimestampComparison(t *testing.T) {
	trans := NewTranslator()

	node := mustParse(t, `created_at >= timestamp("2024-01-01T00:00:00Z")`)
	result, err := trans.Translate(node)
	if err != nil {
		t.Fatalf("Translate() error = %v", err)
	}

	if result == nil {
		t.Fatal("Translate() returned nil")
	}

	if createdAt, ok := result["created_at"].(bson.M); ok {
		if _, hasGte := createdAt["$gte"]; !hasGte {
			t.Error("expected $gte operator for timestamp comparison")
		}
	} else {
		t.Errorf("unexpected result structure: %+v", result)
	}
}

func TestTranslator_NullValue(t *testing.T) {
	trans := NewTranslator()

	node := mustParse(t, `deleted_at == null`)
	result, err := trans.Translate(node)
	if err != nil {
		t.Fatalf("Translate() error = %v", err)
	}

	got := bsonToJSON(result)
	want := `{"deleted_at":null}`
	if got != want {
		t.Errorf("Translate() = %s, want %s", got, want)
	}
}

func TestNewTranslator_DefaultConfig(t *testing.T) {
	trans := NewTranslator()
	if trans == nil {
		t.Fatal("NewTranslator() returned nil")
	}
	if trans.config == nil {
		t.Fatal("NewTranslator() returned translator with nil config")
	}
	if trans.config.MaxDepth() != filter.DefaultMaxDepth {
		t.Errorf("MaxDepth = %d, want %d", trans.config.MaxDepth(), filter.DefaultMaxDepth)
	}
}

func TestTranslatorConfig_ApplyFieldMapping(t *testing.T) {
	cfg := filter.NewTranslatorConfig()
	cfg.SetFieldMapping(map[string]string{
		"userName": "user_name",
	})

	t.Run("mapped field", func(t *testing.T) {
		got := cfg.ApplyFieldMapping("userName")
		if got != "user_name" {
			t.Errorf("ApplyFieldMapping(userName) = %q, want %q", got, "user_name")
		}
	})

	t.Run("unmapped field", func(t *testing.T) {
		got := cfg.ApplyFieldMapping("email")
		if got != "email" {
			t.Errorf("ApplyFieldMapping(email) = %q, want %q", got, "email")
		}
	})

	t.Run("nil mapping", func(t *testing.T) {
		cfg2 := filter.NewTranslatorConfig()
		got := cfg2.ApplyFieldMapping("any")
		if got != "any" {
			t.Errorf("ApplyFieldMapping(any) = %q, want %q", got, "any")
		}
	})
}

func TestTranslatorConfig_IsFieldAllowed(t *testing.T) {
	t.Run("no allowlist", func(t *testing.T) {
		cfg := filter.NewTranslatorConfig()
		if !cfg.IsFieldAllowed("any_field") {
			t.Error("IsFieldAllowed should return true when no allowlist is set")
		}
	})

	t.Run("with allowlist", func(t *testing.T) {
		cfg := filter.NewTranslatorConfig()
		cfg.SetAllowedFields(map[string]struct{}{
			"name": {},
			"age":  {},
		})

		if !cfg.IsFieldAllowed("name") {
			t.Error("IsFieldAllowed(name) should return true")
		}
		if cfg.IsFieldAllowed("email") {
			t.Error("IsFieldAllowed(email) should return false")
		}
	})
}

func TestTranslator_VisitorInterface(t *testing.T) {
	trans := NewTranslator()

	// Verify it implements filter.Visitor
	var _ filter.Visitor = trans
}
