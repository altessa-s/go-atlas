// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"reflect"
	"testing"

	converter "github.com/altessa-s/go-convert"
)

type benchDocModel struct {
	ID    string
	Name  string
	Count int
}

type benchEntity struct {
	ID    string
	Name  string
	Count int
}

// BenchmarkEntityConversion compares the memoized shared-converter path used
// by GetEntity/GetEntities against the former per-call construction (option
// slice + NewShared per query), on an identical convert workload.
func BenchmarkEntityConversion(b *testing.B) {
	m, err := New("bench")
	if err != nil {
		b.Fatal(err)
	}
	model := benchDocModel{ID: "id-1", Name: "name", Count: 42}
	entityType := reflect.TypeFor[benchEntity]()

	b.Run("cached_shared_converter", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			conv := sharedConverter[benchDocModel](&m.repo)
			dst := reflect.New(entityType).Interface()
			conv.Convert(model, dst)
		}
	})

	b.Run("per_call_converter", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			convOpts := append(
				[]converter.Option{converter.WithEmbeddedStructs(converter.EmbeddedFillAlways)},
				m.repo.converterOpts[1:]...,
			)
			conv := converter.New[benchDocModel, any](convOpts...)
			dst := reflect.New(entityType).Interface()
			conv.Convert(model, dst)
		}
	})
}
