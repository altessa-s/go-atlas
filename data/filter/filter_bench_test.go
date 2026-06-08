// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filter_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/data/filter"
)

func BenchmarkParse_Simple(b *testing.B) {
	p, err := filter.NewParser(filter.WithParserNoCache())
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for b.Loop() {
		_, _ = p.Parse(b.Context(), `name == "John"`)
	}
}

func BenchmarkParse_Complex(b *testing.B) {
	p, err := filter.NewParser(filter.WithParserNoCache())
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for b.Loop() {
		_, _ = p.Parse(b.Context(), `(name == "John" || name == "Jane") && age >= 18 && status in ["active", "pending"]`)
	}
}

func BenchmarkEvaluate_Simple(b *testing.B) {
	p, _ := filter.NewParser(filter.WithParserNoCache())
	eval := mustEvaluator(b)
	node, _ := p.Parse(b.Context(), `name == "Alice"`)
	data := map[string]any{"name": "Alice", "age": int64(30)}

	b.ResetTimer()
	for b.Loop() {
		_, _ = eval.Evaluate(node, data)
	}
}

func BenchmarkEvaluate_Complex(b *testing.B) {
	p, _ := filter.NewParser(filter.WithParserNoCache())
	eval := mustEvaluator(b)
	node, _ := p.Parse(b.Context(), `name == "Alice" && age >= 18 && status in ["active", "pending"]`)
	data := map[string]any{"name": "Alice", "age": int64(30), "status": "active"}

	b.ResetTimer()
	for b.Loop() {
		_, _ = eval.Evaluate(node, data)
	}
}

func BenchmarkEvaluate_EnumValues(b *testing.B) {
	p, _ := filter.NewParser(filter.WithParserNoCache())
	eval := mustEvaluator(b, filter.WithEnumValues(map[string][]int64{
		"role": {1, 2, 3, 4, 6, 7},
	}))
	node, _ := p.Parse(b.Context(), `role == 7`)
	data := map[string]any{"role": int64(7)}

	b.ResetTimer()
	for b.Loop() {
		_, _ = eval.Evaluate(node, data)
	}
}

func BenchmarkParseCustomFunction(b *testing.B) {
	p, err := filter.NewParser(
		filter.WithParserNoCache(),
		filter.WithCustomFunctions(map[string]filter.CustomFunction{
			"createdAfter": filter.CompareField("createdAt", filter.OpGT),
			"updatedAfter": filter.CompareField("updatedAt", filter.OpGT),
		}),
	)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for b.Loop() {
		_, _ = p.Parse(b.Context(), `createdAfter("2024-01-01") && updatedAfter("2024-01-02")`)
	}
}
