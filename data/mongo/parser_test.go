// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"reflect"
	"testing"
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

	if meta.StructType != reflect.TypeFor[parserSimpleStruct]() {
		t.Errorf("StructType = %v, want %v", meta.StructType, reflect.TypeFor[parserSimpleStruct]())
	}

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
			if !ok {
				t.Fatalf("field %q not found", tt.field)
			}
			if got := f.fieldValue.Interface(); got != tt.want {
				t.Errorf("value = %v, want %v", got, tt.want)
			}
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
			if !ok {
				t.Fatalf("field %q not found", tt.field)
			}
			if got := f.fieldValue.Interface(); got != tt.want {
				t.Errorf("value = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParser_CacheHit_StructuralMetadataPreserved(t *testing.T) {
	p := NewParser()
	p.ParseStruct(parserSimpleStruct{})

	// fieldName and fieldKind come from cache, not recomputed on every call
	meta := p.ParseStruct(parserSimpleStruct{ID: "x"})
	f, ok := findParserField(meta.Fields, "id")
	if !ok {
		t.Fatal("field \"id\" not found")
	}
	if f.fieldKind != reflect.String {
		t.Errorf("fieldKind = %v, want String", f.fieldKind)
	}
	if f.fieldName != "id" {
		t.Errorf("fieldName = %q, want \"id\"", f.fieldName)
	}
}

func TestParser_FieldIndex_DirectFields(t *testing.T) {
	p := NewParser()
	meta := p.ParseStruct(parserSimpleStruct{ID: "x", Name: "y", Age: 1})

	// Each fieldIndex must match the index returned by FieldByName on the root type
	rootType := reflect.TypeFor[parserSimpleStruct]()
	for _, f := range meta.Fields {
		sf, ok := rootType.FieldByName(f.fieldType.Name)
		if !ok {
			t.Errorf("field %q not found in type via FieldByName", f.fieldType.Name)
			continue
		}
		if !reflect.DeepEqual(f.fieldIndex, sf.Index) {
			t.Errorf("field %q: fieldIndex = %v, want %v", f.fieldName, f.fieldIndex, sf.Index)
		}
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
	if len(meta.Fields) < 3 {
		t.Fatalf("expected at least 3 fields (created_at, updated_at, title), got %d", len(meta.Fields))
	}
	rootType := reflect.TypeFor[parserEmbeddedStruct]()
	for _, f := range meta.Fields {
		sf, ok := rootType.FieldByName(f.fieldType.Name)
		if !ok {
			t.Errorf("field %q not found via FieldByName on root type", f.fieldType.Name)
			continue
		}
		if !reflect.DeepEqual(f.fieldIndex, sf.Index) {
			t.Errorf("embedded field %q: fieldIndex = %v, want %v", f.fieldName, f.fieldIndex, sf.Index)
		}
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
			if !ok {
				t.Fatalf("field %q not found", tt.field)
			}
			if got := f.fieldValue.Interface(); got != tt.want {
				t.Errorf("value = %v, want %v", got, tt.want)
			}
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
			if !ok {
				t.Fatalf("field %q not found", tt.field)
			}
			if f.nestedType != tt.want {
				t.Errorf("nestedType = %v, want %v", f.nestedType, tt.want)
			}
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
			if !ok {
				t.Fatal("field \"nested\" not found")
			}
			if f.nestedType != PointerStruct {
				t.Errorf("nestedType = %v, want PointerStruct", f.nestedType)
			}
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
		if f.nestedType != NoNesting {
			t.Errorf("field %q: nestedType = %v, want NoNesting", f.fieldName, f.nestedType)
		}
	}
}

func TestParser_FieldIndex_PointerEmbeddedFields_FullPathFromRoot(t *testing.T) {
	p := NewParser()
	entity := parserPtrEmbeddedStruct{
		ParserBaseFields: &ParserBaseFields{CreatedAt: 1, UpdatedAt: 2},
		Name:             "alice",
	}
	meta := p.ParseStruct(entity)

	if len(meta.Fields) < 3 {
		t.Fatalf("expected at least 3 fields (created_at, updated_at, name), got %d", len(meta.Fields))
	}
	rootType := reflect.TypeFor[parserPtrEmbeddedStruct]()
	for _, f := range meta.Fields {
		sf, ok := rootType.FieldByName(f.fieldType.Name)
		if !ok {
			t.Errorf("field %q not found via FieldByName on root type", f.fieldType.Name)
			continue
		}
		if !reflect.DeepEqual(f.fieldIndex, sf.Index) {
			t.Errorf("ptr-embedded field %q: fieldIndex = %v, want %v", f.fieldName, f.fieldIndex, sf.Index)
		}
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
			if !ok {
				t.Fatalf("field %q not found", tt.field)
			}
			if got := f.fieldValue.Interface(); got != tt.want {
				t.Errorf("value = %v, want %v", got, tt.want)
			}
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
			if !ok {
				t.Fatal("field \"nested\" not found")
			}
			if f.fieldValue.IsNil() != tt.wantNil {
				t.Errorf("IsNil() = %v, want %v", f.fieldValue.IsNil(), tt.wantNil)
			}
			if !tt.wantNil {
				got := f.fieldValue.Elem().FieldByName("Value").String()
				if got != tt.wantValue {
					t.Errorf("nested.Value = %q, want %q", got, tt.wantValue)
				}
			}
		})
	}
}
