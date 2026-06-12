// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package behavior_test

import (
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/core/types/optional"
	"github.com/altessa-s/go-atlas/domain/behavior"
)

type benchInner struct {
	Secret string `behavior:"input_only"`
	Keep   string
}

type benchResource struct {
	ID         string                    `behavior:"identifier"`
	TenantID   string                    `behavior:"immutable"`
	CreateTime time.Time                 `behavior:"output_only"`
	Name       string                    `behavior:"required"`
	Policy     optional.Optional[string] `behavior:"output_only"`
	Children   []benchInner
}

func newBenchResource() benchResource {
	return benchResource{
		ID:         "id-1",
		TenantID:   "tenant-1",
		CreateTime: time.Unix(0, 0).UTC(),
		Name:       "bucket",
		Policy:     optional.Some("policy"),
		Children:   []benchInner{{Secret: "s", Keep: "k"}, {Secret: "s", Keep: "k"}},
	}
}

func BenchmarkStripUpdate(b *testing.B) {
	for b.Loop() {
		r := newBenchResource()
		if err := behavior.StripUpdate(&r); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStripStrict(b *testing.B) {
	for b.Loop() {
		r := newBenchResource()
		_ = behavior.StripCreate(&r, behavior.WithStrict())
	}
}

func BenchmarkClean(b *testing.B) {
	r := newBenchResource()
	for b.Loop() {
		if _, err := behavior.Clean(r, behavior.WithKinds(behavior.OutputOnly, behavior.Identifier, behavior.Immutable)); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEngineTranslate(b *testing.B) {
	eng := behavior.New[[]string](pathCollector{}, behavior.WithKinds(behavior.OutputOnly))
	for b.Loop() {
		r := newBenchResource()
		if _, err := eng.Translate(b.Context(), &r); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEngineTranslatePooled(b *testing.B) {
	eng := behavior.New[[]string](pathCollector{},
		behavior.WithKinds(behavior.OutputOnly), behavior.WithPooling())
	for b.Loop() {
		r := newBenchResource()
		if _, err := eng.Translate(b.Context(), &r); err != nil {
			b.Fatal(err)
		}
	}
}

// nestedResource routes the walk through nested collection layers: every
// element resolves via a synthetic wrapper Object and the value map exercises
// the copy/write-back path.
type nestedResource struct {
	Grid  [][]benchInner
	ByKey map[string][]benchInner
}

func newNestedResource() nestedResource {
	return nestedResource{
		Grid:  [][]benchInner{{{Secret: "s", Keep: "k"}}, {{Secret: "s", Keep: "k"}, {Secret: "s", Keep: "k"}}},
		ByKey: map[string][]benchInner{"a": {{Secret: "s", Keep: "k"}}, "b": {{Secret: "s", Keep: "k"}}},
	}
}

func BenchmarkStripNestedCollections(b *testing.B) {
	for b.Loop() {
		r := newNestedResource()
		if err := behavior.StripResponse(&r); err != nil {
			b.Fatal(err)
		}
	}
}

// schemaResource holds only absent nested values so the schema-walk has to
// synthesize a representative element for each collection on every call.
type schemaResource struct {
	ID    string `behavior:"identifier"`
	Ptr   *benchInner
	List  []benchInner
	ByKey map[string]benchInner
}

func BenchmarkEngineTranslateSchemaWalk(b *testing.B) {
	eng := behavior.New[[]string](pathCollector{},
		behavior.WithKinds(behavior.InputOnly), behavior.WithSchemaWalk())
	for b.Loop() {
		if _, err := eng.Translate(b.Context(), &schemaResource{}); err != nil {
			b.Fatal(err)
		}
	}
}
