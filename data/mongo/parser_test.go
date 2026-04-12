// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

type parserSimpleStruct struct {
	ID   string `bson:"id"`
	Name string `bson:"name"`
	Age  int    `bson:"age"`
}

// ParserBaseFields is exported so the embedded field is accessible via reflection.
type ParserBaseFields struct {
	CreatedAt int64 `bson:"created_at"`
	UpdatedAt int64 `bson:"updated_at"`
}

type parserEmbeddedStruct struct {
	ParserBaseFields
	Title string `bson:"title"`
}

type parserNestedStruct struct {
	Value string `bson:"value"`
}

type parserPointerStruct struct {
	Name   string              `bson:"name"`
	Nested *parserNestedStruct `bson:"nested"`
}

type parserSliceStruct struct {
	Name  string               `bson:"name"`
	Items []parserNestedStruct `bson:"items"`
}

type parserMapStruct struct {
	Name  string                        `bson:"name"`
	Items map[string]parserNestedStruct `bson:"items"`
}

type parserScalarNestingStruct struct {
	StrPtr  *string           `bson:"str_ptr"`
	Strings []string          `bson:"strings"`
	StrMap  map[string]string `bson:"str_map"`
}

type parserPtrEmbeddedStruct struct {
	*ParserBaseFields
	Name string `bson:"name"`
}

func findParserField(fields []fieldMetadata, name string) (fieldMetadata, bool) {
	for _, f := range fields {
		if f.fieldName == name {
			return f, true
		}
	}
	return fieldMetadata{}, false
}

func TestParser_ParseStruct_Fields(t *testing.T) {
	p := NewParser()
	meta := p.ParseStruct(parserSimpleStruct{ID: "42", Name: "alice", Age: 30})

	require.Equal(t, reflect.TypeFor[parserSimpleStruct](), meta.StructType)

	tests := []struct {
		field string
		want  any
	}{
		{"id", "42"},
		{"name", "alice"},
		{"age", 30},
	}
	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			f, ok := findParserField(meta.Fields, tt.field)
			require.True(t, ok, "field %q not found", tt.field)
			require.Equal(t, tt.want, f.fieldValue.Interface())
		})
	}
}

func TestParser_CacheHit_FieldValuesRefreshed(t *testing.T) {
	p := NewParser()
	p.ParseStruct(parserSimpleStruct{ID: "1", Name: "alice", Age: 10}) // populate cache

	meta := p.ParseStruct(parserSimpleStruct{ID: "2", Name: "bob", Age: 20}) // cache hit

	tests := []struct {
		field string
		want  any
	}{
		{"id", "2"},
		{"name", "bob"},
		{"age", 20},
	}
	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			f, ok := findParserField(meta.Fields, tt.field)
			require.True(t, ok, "field %q not found", tt.field)
			require.Equal(t, tt.want, f.fieldValue.Interface())
		})
	}
}

func TestParser_CacheHit_StructuralMetadataPreserved(t *testing.T) {
	p := NewParser()
	p.ParseStruct(parserSimpleStruct{})

	// fieldName and fieldKind come from cache, not recomputed on every call
	meta := p.ParseStruct(parserSimpleStruct{ID: "x"})
	f, ok := findParserField(meta.Fields, "id")
	require.True(t, ok, "field \"id\" not found")
	require.Equal(t, reflect.String, f.fieldKind)
	require.Equal(t, "id", f.fieldName)
}

func TestParser_FieldIndex_DirectFields(t *testing.T) {
	p := NewParser()
	meta := p.ParseStruct(parserSimpleStruct{ID: "x", Name: "y", Age: 1})

	// Each fieldIndex must match the index returned by FieldByName on the root type
	rootType := reflect.TypeFor[parserSimpleStruct]()
	for _, f := range meta.Fields {
		sf, ok := rootType.FieldByName(f.fieldType.Name)
		require.True(t, ok, "field %q not found in type via FieldByName", f.fieldType.Name)
		require.Equal(t, sf.Index, f.fieldIndex, "field %q: fieldIndex mismatch", f.fieldName)
	}
}

func TestParser_FieldIndex_EmbeddedFields_FullPathFromRoot(t *testing.T) {
	p := NewParser()
	entity := parserEmbeddedStruct{
		ParserBaseFields: ParserBaseFields{CreatedAt: 1, UpdatedAt: 2},
		Title:            "hello",
	}
	meta := p.ParseStruct(entity)

	// Embedded fields must carry the full index path from the root type,
	// not the relative index within the embedded struct
	require.GreaterOrEqual(t, len(meta.Fields), 3, "expected at least 3 fields (created_at, updated_at, title)")
	rootType := reflect.TypeFor[parserEmbeddedStruct]()
	for _, f := range meta.Fields {
		sf, ok := rootType.FieldByName(f.fieldType.Name)
		require.True(t, ok, "field %q not found via FieldByName on root type", f.fieldType.Name)
		require.Equal(t, sf.Index, f.fieldIndex, "embedded field %q: fieldIndex mismatch", f.fieldName)
	}
}

func TestParser_EmbeddedFields_CacheHit_ValuesRefreshed(t *testing.T) {
	p := NewParser()
	p.ParseStruct(parserEmbeddedStruct{
		ParserBaseFields: ParserBaseFields{CreatedAt: 10, UpdatedAt: 20},
		Title:            "first",
	})

	meta := p.ParseStruct(parserEmbeddedStruct{
		ParserBaseFields: ParserBaseFields{CreatedAt: 99, UpdatedAt: 88},
		Title:            "second",
	})

	tests := []struct {
		field string
		want  any
	}{
		{"created_at", int64(99)},
		{"updated_at", int64(88)},
		{"title", "second"},
	}
	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			f, ok := findParserField(meta.Fields, tt.field)
			require.True(t, ok, "field %q not found", tt.field)
			require.Equal(t, tt.want, f.fieldValue.Interface())
		})
	}
}

func TestParser_NestedType_DeterminedByType_NotValue(t *testing.T) {
	tests := []struct {
		name   string
		entity any
		field  string
		want   NestedStructType
	}{
		{"nil pointer", parserPointerStruct{Nested: nil}, "nested", PointerStruct},
		{"non-nil pointer", parserPointerStruct{Nested: &parserNestedStruct{Value: "v"}}, "nested", PointerStruct},
		{"nil slice", parserSliceStruct{Items: nil}, "items", SliceStruct},
		{"non-empty slice", parserSliceStruct{Items: []parserNestedStruct{{Value: "v"}}}, "items", SliceStruct},
		{"nil map", parserMapStruct{Items: nil}, "items", MapStruct},
		{"non-empty map", parserMapStruct{Items: map[string]parserNestedStruct{"k": {Value: "v"}}}, "items", MapStruct},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewParser()
			meta := p.ParseStruct(tt.entity)
			f, ok := findParserField(meta.Fields, tt.field)
			require.True(t, ok, "field %q not found", tt.field)
			require.Equal(t, tt.want, f.nestedType)
		})
	}
}

func TestParser_NestedType_ConsistentAcrossCacheHits(t *testing.T) {
	p := NewParser()

	// First parse with nil pointer — populates cache
	p.ParseStruct(parserPointerStruct{Name: "first", Nested: nil})

	// Subsequent cache hits must return the same nestedType regardless of pointer state
	tests := []struct {
		name   string
		entity parserPointerStruct
	}{
		{"non-nil pointer", parserPointerStruct{Name: "second", Nested: &parserNestedStruct{Value: "v"}}},
		{"nil pointer again", parserPointerStruct{Name: "third", Nested: nil}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta := p.ParseStruct(tt.entity)
			f, ok := findParserField(meta.Fields, "nested")
			require.True(t, ok, "field \"nested\" not found")
			require.Equal(t, PointerStruct, f.nestedType)
		})
	}
}

func TestParser_NestedType_NoNesting_NonStructTypes(t *testing.T) {
	p := NewParser()
	s := "x"
	meta := p.ParseStruct(parserScalarNestingStruct{
		StrPtr:  &s,
		Strings: []string{"a", "b"},
		StrMap:  map[string]string{"k": "v"},
	})

	for _, f := range meta.Fields {
		require.Equal(t, NoNesting, f.nestedType, "field %q", f.fieldName)
	}
}

func TestParser_FieldIndex_PointerEmbeddedFields_FullPathFromRoot(t *testing.T) {
	p := NewParser()
	entity := parserPtrEmbeddedStruct{
		ParserBaseFields: &ParserBaseFields{CreatedAt: 1, UpdatedAt: 2},
		Name:             "alice",
	}
	meta := p.ParseStruct(entity)

	require.GreaterOrEqual(t, len(meta.Fields), 3, "expected at least 3 fields (created_at, updated_at, name)")
	rootType := reflect.TypeFor[parserPtrEmbeddedStruct]()
	for _, f := range meta.Fields {
		sf, ok := rootType.FieldByName(f.fieldType.Name)
		require.True(t, ok, "field %q not found via FieldByName on root type", f.fieldType.Name)
		require.Equal(t, sf.Index, f.fieldIndex, "ptr-embedded field %q: fieldIndex mismatch", f.fieldName)
	}
}

func TestParser_PtrEmbeddedFields_CacheHit_ValuesRefreshed(t *testing.T) {
	p := NewParser()
	p.ParseStruct(parserPtrEmbeddedStruct{
		ParserBaseFields: &ParserBaseFields{CreatedAt: 10, UpdatedAt: 20},
		Name:             "first",
	})

	meta := p.ParseStruct(parserPtrEmbeddedStruct{
		ParserBaseFields: &ParserBaseFields{CreatedAt: 99, UpdatedAt: 88},
		Name:             "second",
	})

	tests := []struct {
		field string
		want  any
	}{
		{"created_at", int64(99)},
		{"updated_at", int64(88)},
		{"name", "second"},
	}
	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			f, ok := findParserField(meta.Fields, tt.field)
			require.True(t, ok, "field %q not found", tt.field)
			require.Equal(t, tt.want, f.fieldValue.Interface())
		})
	}
}

func TestParser_CacheHit_PointerFieldValue_Refreshed(t *testing.T) {
	p := NewParser()
	p.ParseStruct(parserPointerStruct{Name: "first", Nested: &parserNestedStruct{Value: "v1"}})

	tests := []struct {
		name      string
		entity    parserPointerStruct
		wantNil   bool
		wantValue string
	}{
		{"non-nil updated", parserPointerStruct{Name: "second", Nested: &parserNestedStruct{Value: "v2"}}, false, "v2"},
		{"nil refreshed", parserPointerStruct{Name: "third", Nested: nil}, true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta := p.ParseStruct(tt.entity)
			f, ok := findParserField(meta.Fields, "nested")
			require.True(t, ok, "field \"nested\" not found")
			require.Equal(t, tt.wantNil, f.fieldValue.IsNil())
			if !tt.wantNil {
				got := f.fieldValue.Elem().FieldByName("Value").String()
				require.Equal(t, tt.wantValue, got)
			}
		})
	}
}

func TestParser_CacheHit_NestedStructsPopulated(t *testing.T) {
	p := NewParser()
	first := p.ParseStruct(parserPointerStruct{Name: "a", Nested: &parserNestedStruct{Value: "v"}})

	second := p.ParseStruct(parserPointerStruct{Name: "b", Nested: &parserNestedStruct{Value: "w"}})

	nestedType := reflect.TypeFor[parserNestedStruct]()

	require.Contains(t, first.NestedStructs, nestedType, "first parse: NestedStructs missing entry for parserNestedStruct")
	require.Contains(t, second.NestedStructs, nestedType, "cache hit: NestedStructs missing entry for parserNestedStruct")
}

func TestParser_CacheHit_NestedMetadataCleared(t *testing.T) {
	p := NewParser()
	p.ParseStruct(parserPointerStruct{Name: "a", Nested: &parserNestedStruct{Value: "v"}})

	meta := p.ParseStruct(parserPointerStruct{Name: "b", Nested: &parserNestedStruct{Value: "w"}})

	f, ok := findParserField(meta.Fields, "nested")
	require.True(t, ok, "field \"nested\" not found")
	require.Nil(t, f.nestedMetadata, "cache hit: nestedMetadata should be nil")
	require.False(t, f.hasNestedData, "cache hit: hasNestedData should be false")
}

func TestParser_CacheHit_ReturnedCopiesAreIndependent(t *testing.T) {
	p := NewParser()
	p.ParseStruct(parserSimpleStruct{ID: "1", Name: "alice", Age: 10})

	meta1 := p.ParseStruct(parserSimpleStruct{ID: "2", Name: "bob", Age: 20})
	meta2 := p.ParseStruct(parserSimpleStruct{ID: "3", Name: "carol", Age: 30})

	// Verify each result reflects its own entity
	f1, _ := findParserField(meta1.Fields, "name")
	f2, _ := findParserField(meta2.Fields, "name")

	require.Equal(t, "bob", f1.fieldValue.String(), "meta1 name")
	require.Equal(t, "carol", f2.fieldValue.String(), "meta2 name")

	// Verify they are separate slices (mutating one doesn't affect the other)
	require.False(t, &meta1.Fields[0] == &meta2.Fields[0], "meta1.Fields and meta2.Fields share underlying array")
}
