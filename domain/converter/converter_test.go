// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package converter_test

import (
	"reflect"
	"testing"

	"github.com/altessa-s/go-atlas/domain/converter"
)

type Source struct {
	Name     string
	Age      int
	Active   bool
	Ignored  string
	MappedID int
	Tags     []string
	Meta     map[string]string
	Ptr      *int
}

type Dest struct {
	Name     string
	Age      int
	Active   bool
	Ignored  string
	TargetID int
	Tags     []string
	Meta     map[string]string
	Ptr      *int
	Extra    string
}

type NestedSource struct {
	Info Source
}

type NestedDest struct {
	Info Dest
}

func TestConvert(t *testing.T) {
	ptrVal := 42
	src := Source{
		Name:     "Test",
		Age:      30,
		Active:   true,
		Ignored:  "ignore me",
		MappedID: 100,
		Tags:     []string{"a", "b"},
		Meta:     map[string]string{"k": "v"},
		Ptr:      &ptrVal,
	}

	t.Run("Basic Struct Conversion", func(t *testing.T) {
		var dst Dest
		converter.Convert(src, &dst)

		if dst.Name != src.Name {
			t.Errorf("Name mismatch: got %v, want %v", dst.Name, src.Name)
		}
		if dst.Age != src.Age {
			t.Errorf("Age mismatch: got %v, want %v", dst.Age, src.Age)
		}
		if dst.Active != src.Active {
			t.Errorf("Active mismatch: got %v, want %v", dst.Active, src.Active)
		}
		if dst.Ignored != src.Ignored {
			// Without Ignore option, it should be copied
			t.Errorf("Ignored field mismatch: got %v, want %v", dst.Ignored, src.Ignored)
		}
		// MappedID -> TargetID mapping not yet applied, so TargetID should be 0
		if dst.TargetID != 0 {
			t.Errorf("TargetID should be 0, got %v", dst.TargetID)
		}
		if !reflect.DeepEqual(dst.Tags, src.Tags) {
			t.Errorf("Tags mismatch: got %v, want %v", dst.Tags, src.Tags)
		}
		if !reflect.DeepEqual(dst.Meta, src.Meta) {
			t.Errorf("Meta mismatch: got %v, want %v", dst.Meta, src.Meta)
		}
		if dst.Ptr == nil || *dst.Ptr != *src.Ptr {
			t.Errorf("Ptr mismatch: got %v, want %v", dst.Ptr, src.Ptr)
		}
	})

	t.Run("With Options", func(t *testing.T) {
		var dst Dest
		converter.Convert(src, &dst,
			converter.WithIgnoreFields("ignored"),
			converter.WithFieldMappings(map[string]string{"MappedID": "TargetID"}),
		)

		if dst.Ignored != "" {
			t.Errorf("Expected Ignored field to be empty, got %v", dst.Ignored)
		}
		if dst.TargetID != src.MappedID {
			t.Errorf("Expected TargetID to be %v, got %v", src.MappedID, dst.TargetID)
		}
	})

	t.Run("Nested Structs", func(t *testing.T) {
		nSrc := NestedSource{Info: src}
		var nDst NestedDest
		converter.Convert(nSrc, &nDst, converter.WithFieldMappings(map[string]string{"MappedID": "TargetID"}))

		if nDst.Info.Name != src.Name {
			t.Errorf("Nested Name mismatch: got %v, want %v", nDst.Info.Name, src.Name)
		}
		if nDst.Info.TargetID != src.MappedID {
			t.Errorf("Nested TargetID mismatch: got %v, want %v", nDst.Info.TargetID, src.MappedID)
		}
	})

	t.Run("Ignore Zero Values", func(t *testing.T) {
		zeroSrc := Source{Name: "", Age: 0}
		dst := Dest{Name: "Keep", Age: 99}

		converter.Convert(zeroSrc, &dst, converter.WithIgnoreZeroValues())

		if dst.Name != "Keep" {
			t.Error("Expected Name to be kept")
		}
		if dst.Age != 99 {
			t.Error("Expected Age to be kept")
		}
	})

	t.Run("Ignore Nil Values", func(t *testing.T) {
		nilSrc := Source{Ptr: nil}
		existingVal := 10
		dst := Dest{Ptr: &existingVal}

		converter.Convert(nilSrc, &dst, converter.WithIgnoreNilValues())

		if dst.Ptr == nil || *dst.Ptr != 10 {
			t.Error("Expected Ptr to be kept")
		}
	})
}

func TestConvertSlices(t *testing.T) {
	src := []Source{{Name: "A"}, {Name: "B"}}

	t.Run("Slice to Slice", func(t *testing.T) {
		var dst []Dest
		converter.Convert(src, &dst)

		if len(dst) != 2 {
			t.Fatalf("Expected 2 elements, got %d", len(dst))
		}
		if dst[0].Name != "A" || dst[1].Name != "B" {
			t.Error("Element mismatch")
		}
	})
}

func TestConvertSeq(t *testing.T) {
	src := []Source{{Name: "A"}, {Name: "B"}}
	iter := converter.ConvertSeq[Source, Dest](src)

	count := 0
	for item := range iter {
		if item.Name != src[count].Name {
			t.Errorf("Item %d mismatch", count)
		}
		count++
	}
	if count != 2 {
		t.Errorf("Expected 2 items, got %d", count)
	}
}

func TestConvertMapSeq(t *testing.T) {
	src := map[string]int{"one": 1, "two": 2}
	// Converting Int values to Int values (direct assignable) with Any->Any

	// Note: ConvertMapSeq is generic on K1,V1 but internal implementation seems to support any->any via reflection mapping?
	// The function signature is ConvertMapSeq[K1, V1, K2, V2].

	iter := converter.ConvertMapSeq[string, int, string, int](src)

	res := make(map[string]int)
	for k, v := range iter {
		res[k] = v
	}

	if len(res) != 2 || res["one"] != 1 || res["two"] != 2 {
		t.Errorf("Map conversion failed: %v", res)
	}
}

type SmallStruct struct {
	A int
	B int
}

func TestStackAllocationOptimization(t *testing.T) {
	// Logic check: The converter uses stack allocation for <= 4 fields.
	// We verify that it works correctly for small structs.
	src := SmallStruct{A: 1, B: 2}
	var dst SmallStruct

	converter.Convert(src, &dst)

	if dst.A != 1 || dst.B != 2 {
		t.Errorf("Small struct conversion failed: %v", dst)
	}
}

type EmbeddedSrc struct {
	SmallStruct
	Extra string
}

type EmbeddedDst struct {
	SmallStruct
	Extra string
}

func TestEmbeddedStructs(t *testing.T) {
	src := EmbeddedSrc{
		SmallStruct: SmallStruct{A: 10, B: 20},
		Extra:       "ext",
	}

	t.Run("Default (No Handle Embedded)", func(t *testing.T) {
		// Without option, embedded fields are treated as normal fields struct-to-struct
		// Since names match (SmallStruct), it should work if types match.
		var dst EmbeddedDst
		converter.Convert(src, &dst)

		if dst.A != 10 || dst.B != 20 {
			t.Errorf("Default embedded handling failed: %v", dst)
		}
	})

	t.Run("With Handle Embedded", func(t *testing.T) {
		var dst EmbeddedDst
		converter.Convert(src, &dst, converter.WithHandleEmbeddedStructs(true))

		if dst.A != 10 || dst.B != 20 {
			t.Errorf("Explicit embedded handling failed: %v", dst)
		}
	})
}

func TestNew(t *testing.T) {
	conv := converter.New[Source, *Dest]()
	if conv == nil {
		t.Fatal("expected non-nil converter")
	}
	src := Source{Name: "test", Age: 25}
	var dst Dest
	conv.Convert(src, &dst)
	if dst.Name != "test" || dst.Age != 25 {
		t.Errorf("got Name=%q Age=%d", dst.Name, dst.Age)
	}
}

func TestNewAny(t *testing.T) {
	conv := converter.NewAny()
	if conv == nil {
		t.Fatal("expected non-nil converter")
	}
}

func TestWithIgnoreNilValues(t *testing.T) {
	src := Source{Ptr: nil, Name: "test"}
	dst := Dest{Name: "original"}
	pv := 99
	dst.Ptr = &pv

	converter.Convert(src, &dst, converter.WithIgnoreNilValues())
	if dst.Ptr == nil {
		t.Fatal("expected Ptr to remain non-nil with IgnoreNilValues")
	}
	if *dst.Ptr != 99 {
		t.Fatalf("expected Ptr value 99, got %d", *dst.Ptr)
	}
}

func TestWithIgnoreZeroValues(t *testing.T) {
	src := Source{Name: "", Age: 0}
	dst := Dest{Name: "original", Age: 42}

	converter.Convert(src, &dst, converter.WithIgnoreZeroValues())
	if dst.Name != "original" {
		t.Fatalf("expected Name to remain 'original', got %q", dst.Name)
	}
	if dst.Age != 42 {
		t.Fatalf("expected Age to remain 42, got %d", dst.Age)
	}
}

func TestIndirectType(t *testing.T) {
	typ := reflect.TypeFor[*Source]()
	indirect := converter.IndirectType(typ)
	if indirect.Kind() != reflect.Struct {
		t.Fatalf("expected Struct, got %v", indirect.Kind())
	}
	if indirect.Name() != "Source" {
		t.Fatalf("expected Source, got %s", indirect.Name())
	}

	// Non-pointer should return same type
	strType := reflect.TypeFor[string]()
	if got := converter.IndirectType(strType); got != strType {
		t.Fatal("expected same type for non-pointer")
	}
}

func TestIsPrimitive(t *testing.T) {
	tests := []struct {
		kind reflect.Kind
		want bool
	}{
		{reflect.String, true},
		{reflect.Int, true},
		{reflect.Float64, true},
		{reflect.Bool, true},
		{reflect.Struct, false},
		{reflect.Slice, false},
		{reflect.Map, false},
	}
	for _, tt := range tests {
		if got := converter.IsPrimitive(tt.kind); got != tt.want {
			t.Errorf("IsPrimitive(%v) = %v, want %v", tt.kind, got, tt.want)
		}
	}
}
