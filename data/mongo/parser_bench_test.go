// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import "testing"

func BenchmarkParserCacheHitSimple(b *testing.B) {
	p := NewParser()
	entity := parserSimpleStruct{ID: "1", Name: "n", Age: 1}
	p.ParseStruct(&entity)

	for b.Loop() {
		p.ParseStruct(&entity)
	}
}

func BenchmarkParserCacheHitEmbedded(b *testing.B) {
	p := NewParser()
	entity := parserEmbeddedStruct{ParserBaseFields: ParserBaseFields{CreatedAt: 1, UpdatedAt: 2}, Title: "t"}
	p.ParseStruct(&entity)

	for b.Loop() {
		p.ParseStruct(&entity)
	}
}

func BenchmarkParserCacheHitNestedPointer(b *testing.B) {
	p := NewParser()
	entity := parserPointerStruct{Name: "n", Nested: &parserNestedStruct{Value: "v"}}
	p.ParseStruct(&entity)

	for b.Loop() {
		p.ParseStruct(&entity)
	}
}

func BenchmarkParserColdParse(b *testing.B) {
	for b.Loop() {
		p := NewParser()
		entity := parserSimpleStruct{ID: "1", Name: "n", Age: 1}
		p.ParseStruct(&entity)
	}
}
