// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filter_test

import (
	"errors"
	"testing"

	"github.com/altessa-s/go-atlas/data/filter"
)

func newTestParser(t *testing.T) *filter.Parser {
	t.Helper()
	p, err := filter.NewParser(filter.WithParserNoCache())
	if err != nil {
		t.Fatalf("NewParser: %v", err)
	}
	return p
}

func TestEvaluator_Comparison(t *testing.T) {
	p := newTestParser(t)
	eval := filter.NewEvaluator()

	data := map[string]any{
		"name":   "Alice",
		"age":    int64(30),
		"active": true,
	}

	tests := []struct {
		expr string
		want bool
	}{
		{`name == "Alice"`, true},
		{`name == "Bob"`, false},
		{`name != "Bob"`, true},
		{`age == 30`, true},
		{`age > 25`, true},
		{`age < 25`, false},
		{`age >= 30`, true},
		{`age <= 30`, true},
		{`active == true`, true},
		{`active != false`, true},
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			node, err := p.Parse(t.Context(), tt.expr)
			if err != nil {
				t.Fatalf("Parse(%q): %v", tt.expr, err)
			}
			got, err := eval.Evaluate(node, data)
			if err != nil {
				t.Fatalf("Evaluate(%q): %v", tt.expr, err)
			}
			if got != tt.want {
				t.Errorf("Evaluate(%q) = %v, want %v", tt.expr, got, tt.want)
			}
		})
	}
}

func TestEvaluator_Logical(t *testing.T) {
	p := newTestParser(t)
	eval := filter.NewEvaluator()

	data := map[string]any{
		"status": "active",
		"count":  int64(5),
	}

	tests := []struct {
		expr string
		want bool
	}{
		{`status == "active" && count > 3`, true},
		{`status == "active" && count > 10`, false},
		{`status == "paused" || count > 3`, true},
		{`status == "paused" || count > 10`, false},
		{`!(status == "paused")`, true},
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			node, err := p.Parse(t.Context(), tt.expr)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			got, err := eval.Evaluate(node, data)
			if err != nil {
				t.Fatalf("Evaluate: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEvaluator_StringFunctions(t *testing.T) {
	p := newTestParser(t)
	eval := filter.NewEvaluator()

	data := map[string]any{"name": "hello-world"}

	tests := []struct {
		expr string
		want bool
	}{
		{`name.contains("world")`, true},
		{`name.contains("xyz")`, false},
		{`name.startsWith("hello")`, true},
		{`name.startsWith("world")`, false},
		{`name.endsWith("world")`, true},
		{`name.endsWith("hello")`, false},
		{`name.matches("^hello.*")`, true},
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			node, err := p.Parse(t.Context(), tt.expr)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			got, err := eval.Evaluate(node, data)
			if err != nil {
				t.Fatalf("Evaluate: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEvaluator_In(t *testing.T) {
	p := newTestParser(t)
	eval := filter.NewEvaluator()

	data := map[string]any{"status": "active"}

	tests := []struct {
		expr string
		want bool
	}{
		{`status in ["active", "running"]`, true},
		{`status in ["paused", "disabled"]`, false},
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			node, err := p.Parse(t.Context(), tt.expr)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			got, err := eval.Evaluate(node, data)
			if err != nil {
				t.Fatalf("Evaluate: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEvaluator_Has(t *testing.T) {
	p := newTestParser(t)
	eval := filter.NewEvaluator()

	data := map[string]any{
		"name": "test",
		"nested": map[string]any{
			"field": "value",
		},
	}

	tests := []struct {
		expr string
		want bool
	}{
		{`has(data.name)`, true},
		{`has(data.missing)`, false},
		{`has(data.nested)`, true},
	}

	// Wrap data under a "data" key so has() macro works with field selection.
	data = map[string]any{"data": data}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			node, err := p.Parse(t.Context(), tt.expr)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			got, err := eval.Evaluate(node, data)
			if err != nil {
				t.Fatalf("Evaluate: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEvaluator_NestedFields(t *testing.T) {
	p := newTestParser(t)
	eval := filter.NewEvaluator()

	data := map[string]any{
		"address": map[string]any{
			"city": "NYC",
		},
	}

	node, err := p.Parse(t.Context(), `address.city == "NYC"`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got, err := eval.Evaluate(node, data)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !got {
		t.Error("expected true")
	}
}

func TestEvaluator_AllowedFields(t *testing.T) {
	p := newTestParser(t)
	eval := filter.NewEvaluator(filter.WithAllowedFields("name"))

	data := map[string]any{"name": "test", "secret": "hidden"}

	node, err := p.Parse(t.Context(), `secret == "hidden"`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	_, err = eval.Evaluate(node, data)
	if err == nil {
		t.Fatal("expected error for disallowed field")
	}
}

func TestEvaluator_FieldMapping(t *testing.T) {
	p := newTestParser(t)
	eval := filter.NewEvaluator(filter.WithFieldMapping(map[string]string{
		"userName": "user_name",
	}))

	data := map[string]any{"user_name": "alice"}

	node, err := p.Parse(t.Context(), `userName == "alice"`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got, err := eval.Evaluate(node, data)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !got {
		t.Error("expected true")
	}
}

func TestEvaluator_Size(t *testing.T) {
	p := newTestParser(t)
	eval := filter.NewEvaluator()

	data := map[string]any{"name": "hello"}

	node, err := p.Parse(t.Context(), `name.size() == 5`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got, err := eval.Evaluate(node, data)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !got {
		t.Error("expected true")
	}
}

func TestEvaluator_NilComparison(t *testing.T) {
	p := newTestParser(t)
	eval := filter.NewEvaluator()

	data := map[string]any{"name": "test"}

	node, err := p.Parse(t.Context(), `missing == null`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got, err := eval.Evaluate(node, data)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !got {
		t.Error("expected true for missing field == null")
	}
}

func TestEvaluator_RegexLengthLimit(t *testing.T) {
	p := newTestParser(t)
	data := map[string]any{"name": "hello"}

	t.Run("short regex is accepted", func(t *testing.T) {
		eval := filter.NewEvaluator()
		node, err := p.Parse(t.Context(), `name.matches("^hello")`)
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		got, err := eval.Evaluate(node, data)
		if err != nil {
			t.Fatalf("Evaluate: %v", err)
		}
		if !got {
			t.Error("expected true")
		}
	})

	t.Run("regex exceeding default limit is rejected", func(t *testing.T) {
		eval := filter.NewEvaluator()
		// Build expression with a regex pattern exceeding 1024 bytes
		longPattern := make([]byte, 1025)
		for i := range longPattern {
			longPattern[i] = 'a'
		}
		node := &filter.CallNode{
			Op:     filter.OpMatches,
			Target: &filter.IdentNode{Name: "name"},
			Args:   []filter.Node{&filter.LiteralNode{Value: string(longPattern)}},
		}
		_, err := eval.Evaluate(node, data)
		if err == nil {
			t.Fatal("expected error for regex exceeding max length")
		}
		if !errors.Is(err, filter.ErrInvalidRegex) {
			t.Errorf("expected ErrInvalidRegex, got: %v", err)
		}
	})

	t.Run("custom regex length limit", func(t *testing.T) {
		eval := filter.NewEvaluator(filter.WithMaxRegexLength(10))
		node := &filter.CallNode{
			Op:     filter.OpMatches,
			Target: &filter.IdentNode{Name: "name"},
			Args:   []filter.Node{&filter.LiteralNode{Value: "a]long-pattern"}},
		}
		_, err := eval.Evaluate(node, data)
		if err == nil {
			t.Fatal("expected error for regex exceeding custom max length")
		}
		if !errors.Is(err, filter.ErrInvalidRegex) {
			t.Errorf("expected ErrInvalidRegex, got: %v", err)
		}
	})
}

func TestEvaluator_MaxOperations(t *testing.T) {
	p := newTestParser(t)
	data := map[string]any{"a": int64(1), "b": int64(2), "c": int64(3)}

	t.Run("normal expression within limit", func(t *testing.T) {
		eval := filter.NewEvaluator()
		node, err := p.Parse(t.Context(), `a == 1 && b == 2`)
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		got, err := eval.Evaluate(node, data)
		if err != nil {
			t.Fatalf("Evaluate: %v", err)
		}
		if !got {
			t.Error("expected true")
		}
	})

	t.Run("expression exceeding low limit is rejected", func(t *testing.T) {
		eval := filter.NewEvaluator(filter.WithMaxOperations(3))
		// a == 1 && b == 2 visits: BinaryOp(&&), BinaryOp(==), Ident(a), Literal(1), BinaryOp(==), ...
		// With limit=3, it should fail after 3 operations
		node, err := p.Parse(t.Context(), `a == 1 && b == 2`)
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		_, err = eval.Evaluate(node, data)
		if err == nil {
			t.Fatal("expected error for exceeding max operations")
		}
		if !errors.Is(err, filter.ErrMaxOperationsExceeded) {
			t.Errorf("expected ErrMaxOperationsExceeded, got: %v", err)
		}
	})
}
