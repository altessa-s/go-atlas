// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package lua

import (
	"errors"
	"testing"

	"github.com/altessa-s/go-atlas/data/filter"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

func TestTranslator_BasicComparisons(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want string
	}{
		{
			name: "equal int",
			expr: `field == 42`,
			want: `(d["field"] == 42)`,
		},
		{
			name: "equal string",
			expr: `field == "val"`,
			want: `(d["field"] == "val")`,
		},
		{
			name: "equal bool true",
			expr: `field == true`,
			want: `(d["field"] == true)`,
		},
		{
			name: "equal bool false",
			expr: `field == false`,
			want: `(d["field"] == false)`,
		},
		{
			name: "equal null",
			expr: `field == null`,
			want: `(d["field"] == nil)`,
		},
		{
			name: "not equal",
			expr: `field != 0`,
			want: `(d["field"] ~= 0)`,
		},
		{
			name: "greater than",
			expr: `field > 10`,
			want: `(d["field"] > 10)`,
		},
		{
			name: "greater than or equal",
			expr: `field >= 10`,
			want: `(d["field"] >= 10)`,
		},
		{
			name: "less than",
			expr: `field < 10`,
			want: `(d["field"] < 10)`,
		},
		{
			name: "less than or equal",
			expr: `field <= 10`,
			want: `(d["field"] <= 10)`,
		},
	}

	trans := NewTranslator("")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := testhelpers.MustParseFilter(t, tt.expr)
			got, err := trans.Translate(node)
			if err != nil {
				t.Fatalf("Translate() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Translate() = %q, want %q", got, tt.want)
			}
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
			expr: `a == 1 && b == 2`,
			want: `((d["a"] == 1) and (d["b"] == 2))`,
		},
		{
			name: "or",
			expr: `a == 1 || b == 2`,
			want: `((d["a"] == 1) or (d["b"] == 2))`,
		},
		{
			name: "not ident",
			expr: `!active`,
			want: `(d["active"] ~= true)`,
		},
		{
			name: "not complex",
			expr: `!(status == 1)`,
			want: `(not (d["status"] == 1))`,
		},
	}

	trans := NewTranslator("")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := testhelpers.MustParseFilter(t, tt.expr)
			got, err := trans.Translate(node)
			if err != nil {
				t.Fatalf("Translate() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Translate() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTranslator_NestedFields(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want string
	}{
		{
			name: "two levels",
			expr: `addr.city == "NYC"`,
			want: `(d["addr"]["city"] == "NYC")`,
		},
		{
			name: "three levels",
			expr: `a.b.c == 1`,
			want: `(d["a"]["b"]["c"] == 1)`,
		},
	}

	trans := NewTranslator("")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := testhelpers.MustParseFilter(t, tt.expr)
			got, err := trans.Translate(node)
			if err != nil {
				t.Fatalf("Translate() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Translate() = %q, want %q", got, tt.want)
			}
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
			expr: `field in ["a", "b"]`,
			want: `(d["field"] == "a" or d["field"] == "b")`,
		},
		{
			name: "single value",
			expr: `field in ["only"]`,
			want: `(d["field"] == "only")`,
		},
	}

	trans := NewTranslator("")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := testhelpers.MustParseFilter(t, tt.expr)
			got, err := trans.Translate(node)
			if err != nil {
				t.Fatalf("Translate() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Translate() = %q, want %q", got, tt.want)
			}
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
			expr: `field.contains("sub")`,
			want: `(string.find(d["field"], "sub", 1, true) ~= nil)`,
		},
		{
			name: "startsWith",
			expr: `field.startsWith("pre")`,
			want: `(string.sub(d["field"], 1, 3) == "pre")`,
		},
		{
			name: "endsWith",
			expr: `field.endsWith("suf")`,
			want: `(string.sub(d["field"], -3) == "suf")`,
		},
	}

	trans := NewTranslator("")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := testhelpers.MustParseFilter(t, tt.expr)
			got, err := trans.Translate(node)
			if err != nil {
				t.Fatalf("Translate() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Translate() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTranslator_UnsupportedOperations(t *testing.T) {
	trans := NewTranslator("")

	node := testhelpers.MustParseFilter(t, `name.matches("^A.*")`)
	_, err := trans.Translate(node)
	if !errors.Is(err, filter.ErrUnsupportedOperation) {
		t.Errorf("Translate(matches) error = %v, want %v", err, filter.ErrUnsupportedOperation)
	}
}

func TestTranslator_HasFunction(t *testing.T) {
	trans := NewTranslator("")

	node := testhelpers.MustParseFilter(t, `has(user.email)`)
	got, err := trans.Translate(node)
	if err != nil {
		t.Fatalf("Translate() error = %v", err)
	}
	want := `(d["user"]["email"] ~= nil)`
	if got != want {
		t.Errorf("Translate() = %q, want %q", got, want)
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
			want: `(#d["tags"] == 3)`,
		},
		{
			name: "size greater than",
			expr: `tags.size() > 0`,
			want: `(#d["tags"] > 0)`,
		},
	}

	trans := NewTranslator("")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := testhelpers.MustParseFilter(t, tt.expr)
			got, err := trans.Translate(node)
			if err != nil {
				t.Fatalf("Translate() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Translate() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTranslator_NilNode(t *testing.T) {
	trans := NewTranslator("")

	got, err := trans.Translate(nil)
	if err != nil {
		t.Fatalf("Translate(nil) error = %v", err)
	}
	if got != "true" {
		t.Errorf("Translate(nil) = %q, want %q", got, "true")
	}
}

func TestTranslator_WithAllowedFields(t *testing.T) {
	trans := NewTranslator("", filter.WithAllowedFields("name", "age"))

	t.Run("allowed field", func(t *testing.T) {
		node := testhelpers.MustParseFilter(t, `name == "John"`)
		_, err := trans.Translate(node)
		if err != nil {
			t.Errorf("Translate() error = %v for allowed field", err)
		}
	})

	t.Run("disallowed field", func(t *testing.T) {
		node := testhelpers.MustParseFilter(t, `email == "test@example.com"`)
		_, err := trans.Translate(node)
		if !errors.Is(err, filter.ErrFieldNotAllowed) {
			t.Errorf("Translate() error = %v, want %v", err, filter.ErrFieldNotAllowed)
		}
	})
}

func TestTranslator_WithFieldMapping(t *testing.T) {
	trans := NewTranslator("",
		filter.WithFieldMapping(map[string]string{
			"userName": "user_name",
		}),
	)

	node := testhelpers.MustParseFilter(t, `userName == "John"`)
	got, err := trans.Translate(node)
	if err != nil {
		t.Fatalf("Translate() error = %v", err)
	}
	want := `(d["user_name"] == "John")`
	if got != want {
		t.Errorf("Translate() = %q, want %q", got, want)
	}
}

func TestTranslator_WithMaxDepth(t *testing.T) {
	trans := NewTranslator("", filter.WithMaxDepth(2))

	t.Run("within depth", func(t *testing.T) {
		node := testhelpers.MustParseFilter(t, `a == 1 && b == 2`)
		_, err := trans.Translate(node)
		if err != nil {
			t.Errorf("Translate() error = %v for valid depth", err)
		}
	})

	t.Run("exceeds depth", func(t *testing.T) {
		node := testhelpers.MustParseFilter(t, `(a == 1 && b == 2) && (c == 3 && d == 4)`)
		_, err := trans.Translate(node)
		if !errors.Is(err, filter.ErrMaxDepthExceeded) {
			t.Errorf("Translate() error = %v, want %v", err, filter.ErrMaxDepthExceeded)
		}
	})
}

func TestTranslator_CustomTableVar(t *testing.T) {
	trans := NewTranslator("item")

	node := testhelpers.MustParseFilter(t, `name == "test"`)
	got, err := trans.Translate(node)
	if err != nil {
		t.Fatalf("Translate() error = %v", err)
	}
	want := `(item["name"] == "test")`
	if got != want {
		t.Errorf("Translate() = %q, want %q", got, want)
	}
}

func TestTranslator_StringEscaping(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want string
	}{
		{
			name: "double quotes",
			expr: `field == "say \"hello\""`,
			want: `(d["field"] == "say \"hello\"")`,
		},
		{
			name: "backslash",
			expr: `field == "path\\to"`,
			want: `(d["field"] == "path\\to")`,
		},
	}

	trans := NewTranslator("")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := testhelpers.MustParseFilter(t, tt.expr)
			got, err := trans.Translate(node)
			if err != nil {
				t.Fatalf("Translate() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Translate() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTranslator_ComplexExpressions(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want string
	}{
		{
			name: "and with in",
			expr: `status == 1 && (role in ["admin", "editor"])`,
			want: `((d["status"] == 1) and (d["role"] == "admin" or d["role"] == "editor"))`,
		},
		{
			name: "triple and",
			expr: `a == 1 && b == 2 && c == 3`,
			want: `(((d["a"] == 1) and (d["b"] == 2)) and (d["c"] == 3))`,
		},
	}

	trans := NewTranslator("")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := testhelpers.MustParseFilter(t, tt.expr)
			got, err := trans.Translate(node)
			if err != nil {
				t.Fatalf("Translate() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Translate() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTranslator_VisitorInterface(t *testing.T) {
	trans := NewTranslator("")
	var _ filter.Visitor = trans
}

func TestNewTranslator_DefaultConfig(t *testing.T) {
	trans := NewTranslator("")
	if trans == nil {
		t.Fatal("NewTranslator() returned nil")
	}
	if trans.config == nil {
		t.Fatal("NewTranslator() returned translator with nil config")
	}
	if trans.config.MaxDepth() != filter.DefaultMaxDepth {
		t.Errorf("MaxDepth = %d, want %d", trans.config.MaxDepth(), filter.DefaultMaxDepth)
	}
	if trans.tableVar != "d" {
		t.Errorf("tableVar = %q, want %q", trans.tableVar, "d")
	}
}
