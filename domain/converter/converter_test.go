// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package converter_test

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

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

		require.Equal(t, src.Name, dst.Name)
		require.Equal(t, src.Age, dst.Age)
		require.Equal(t, src.Active, dst.Active)
		// Without Ignore option, it should be copied
		require.Equal(t, src.Ignored, dst.Ignored)
		// MappedID -> TargetID mapping not yet applied, so TargetID should be 0
		require.Equal(t, 0, dst.TargetID)
		require.Equal(t, src.Tags, dst.Tags)
		require.Equal(t, src.Meta, dst.Meta)
		require.NotNil(t, dst.Ptr)
		require.Equal(t, *src.Ptr, *dst.Ptr)
	})

	t.Run("With Options", func(t *testing.T) {
		var dst Dest
		converter.Convert(src, &dst,
			converter.WithIgnoreFields("ignored"),
			converter.WithFieldMappings(map[string]string{"MappedID": "TargetID"}),
		)

		require.Equal(t, "", dst.Ignored)
		require.Equal(t, src.MappedID, dst.TargetID)
	})

	t.Run("Nested Structs", func(t *testing.T) {
		nSrc := NestedSource{Info: src}
		var nDst NestedDest
		converter.Convert(nSrc, &nDst, converter.WithFieldMappings(map[string]string{"MappedID": "TargetID"}))

		require.Equal(t, src.Name, nDst.Info.Name)
		require.Equal(t, src.MappedID, nDst.Info.TargetID)
	})

	t.Run("Ignore Zero Values", func(t *testing.T) {
		zeroSrc := Source{Name: "", Age: 0}
		dst := Dest{Name: "Keep", Age: 99}

		converter.Convert(zeroSrc, &dst, converter.WithIgnoreZeroValues())

		require.Equal(t, "Keep", dst.Name)
		require.Equal(t, 99, dst.Age)
	})

	t.Run("Ignore Nil Values", func(t *testing.T) {
		nilSrc := Source{Ptr: nil}
		existingVal := 10
		dst := Dest{Ptr: &existingVal}

		converter.Convert(nilSrc, &dst, converter.WithIgnoreNilValues())

		require.NotNil(t, dst.Ptr)
		require.Equal(t, 10, *dst.Ptr)
	})
}

func TestConvertSlices(t *testing.T) {
	src := []Source{{Name: "A"}, {Name: "B"}}

	t.Run("Slice to Slice", func(t *testing.T) {
		var dst []Dest
		converter.Convert(src, &dst)

		require.Len(t, dst, 2)
		require.Equal(t, "A", dst[0].Name)
		require.Equal(t, "B", dst[1].Name)
	})
}

func TestConvertSeq(t *testing.T) {
	src := []Source{{Name: "A"}, {Name: "B"}}
	iter := converter.ConvertSeq[Source, Dest](src)

	count := 0
	for item := range iter {
		require.Equal(t, src[count].Name, item.Name)
		count++
	}
	require.Equal(t, 2, count)
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

	require.Equal(t, map[string]int{"one": 1, "two": 2}, res)
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

	require.Equal(t, 1, dst.A)
	require.Equal(t, 2, dst.B)
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

		require.Equal(t, 10, dst.A)
		require.Equal(t, 20, dst.B)
	})

	t.Run("With Handle Embedded", func(t *testing.T) {
		var dst EmbeddedDst
		converter.Convert(src, &dst, converter.WithHandleEmbeddedStructs(true))

		require.Equal(t, 10, dst.A)
		require.Equal(t, 20, dst.B)
	})
}

func TestNew(t *testing.T) {
	conv := converter.New[Source, *Dest]()
	require.NotNil(t, conv)
	src := Source{Name: "test", Age: 25}
	var dst Dest
	conv.Convert(src, &dst)
	require.Equal(t, "test", dst.Name)
	require.Equal(t, 25, dst.Age)
}

func TestNewAny(t *testing.T) {
	conv := converter.NewAny()
	require.NotNil(t, conv)
}

func TestWithIgnoreNilValues(t *testing.T) {
	src := Source{Ptr: nil, Name: "test"}
	dst := Dest{Name: "original"}
	pv := 99
	dst.Ptr = &pv

	converter.Convert(src, &dst, converter.WithIgnoreNilValues())
	require.NotNil(t, dst.Ptr)
	require.Equal(t, 99, *dst.Ptr)
}

func TestWithIgnoreZeroValues(t *testing.T) {
	src := Source{Name: "", Age: 0}
	dst := Dest{Name: "original", Age: 42}

	converter.Convert(src, &dst, converter.WithIgnoreZeroValues())
	require.Equal(t, "original", dst.Name)
	require.Equal(t, 42, dst.Age)
}

func TestIndirectType(t *testing.T) {
	typ := reflect.TypeFor[*Source]()
	indirect := converter.IndirectType(typ)
	require.Equal(t, reflect.Struct, indirect.Kind())
	require.Equal(t, "Source", indirect.Name())

	// Non-pointer should return same type
	strType := reflect.TypeFor[string]()
	require.Equal(t, strType, converter.IndirectType(strType))
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
		require.Equal(t, tt.want, converter.IsPrimitive(tt.kind), "IsPrimitive(%v)", tt.kind)
	}
}
