// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filter_test

import (
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
			node, err := p.Parse(tt.expr)
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
			node, err := p.Parse(tt.expr)
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
			node, err := p.Parse(tt.expr)
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
			node, err := p.Parse(tt.expr)
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
			node, err := p.Parse(tt.expr)
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

	node, err := p.Parse(`address.city == "NYC"`)
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

	node, err := p.Parse(`secret == "hidden"`)
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

	node, err := p.Parse(`userName == "alice"`)
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

	node, err := p.Parse(`name.size() == 5`)
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

	node, err := p.Parse(`missing == null`)
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
