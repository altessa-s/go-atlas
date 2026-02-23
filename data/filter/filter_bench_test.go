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
		_, _ = p.Parse(`name == "John"`)
	}
}

func BenchmarkParse_Complex(b *testing.B) {
	p, err := filter.NewParser(filter.WithParserNoCache())
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for b.Loop() {
		_, _ = p.Parse(`(name == "John" || name == "Jane") && age >= 18 && status in ["active", "pending"]`)
	}
}

func BenchmarkEvaluate_Simple(b *testing.B) {
	p, _ := filter.NewParser(filter.WithParserNoCache())
	eval := filter.NewEvaluator()
	node, _ := p.Parse(`name == "Alice"`)
	data := map[string]any{"name": "Alice", "age": int64(30)}

	b.ResetTimer()
	for b.Loop() {
		_, _ = eval.Evaluate(node, data)
	}
}

func BenchmarkEvaluate_Complex(b *testing.B) {
	p, _ := filter.NewParser(filter.WithParserNoCache())
	eval := filter.NewEvaluator()
	node, _ := p.Parse(`name == "Alice" && age >= 18 && status in ["active", "pending"]`)
	data := map[string]any{"name": "Alice", "age": int64(30), "status": "active"}

	b.ResetTimer()
	for b.Loop() {
		_, _ = eval.Evaluate(node, data)
	}
}
