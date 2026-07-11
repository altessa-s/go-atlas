// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package converter_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/domain/converter"

	convcodec "github.com/altessa-s/go-atlas/domain/converter/codec"
)

type SrcUser struct {
	Name  string
	Email string
	Age   int
}

type DstUser struct {
	Name  string
	Email string
	Age   int
}

func TestConvert_Basic(t *testing.T) {
	src := &SrcUser{Name: "Alice", Email: "a@b.com", Age: 30}
	dst := &DstUser{}

	converter.Convert(src, dst)

	require.Equal(t, "Alice", dst.Name)
	require.Equal(t, "a@b.com", dst.Email)
	require.Equal(t, 30, dst.Age)
}

func TestConvert_IgnoreZeroValues(t *testing.T) {
	src := &SrcUser{Name: "Bob", Age: 0}
	dst := &DstUser{Name: "Original", Age: 25}

	converter.Convert(src, dst, converter.WithIgnoreZeroValues())

	require.Equal(t, "Bob", dst.Name)
	require.Equal(t, 25, dst.Age, "zero should be ignored")
}

func TestConvert_IgnoreFields(t *testing.T) {
	src := &SrcUser{Name: "Carol", Email: "c@d.com", Age: 40}
	dst := &DstUser{}

	converter.Convert(src, dst, converter.WithIgnoreFields("Email"))

	require.Equal(t, "Carol", dst.Name)
	require.Equal(t, "", dst.Email, "should be ignored")
}

func TestConvert_FieldMappings(t *testing.T) {
	type Src struct {
		FullName string
	}
	type Dst struct {
		Name string
	}

	src := &Src{FullName: "Dave"}
	dst := &Dst{}

	converter.Convert(src, dst, converter.WithFieldMappings(map[string]string{"FullName": "Name"}))

	require.Equal(t, "Dave", dst.Name)
}

func TestNew_Converter(t *testing.T) {
	conv := converter.New[*SrcUser, *DstUser]()
	require.NotNil(t, conv)

	src := &SrcUser{Name: "Eve", Age: 20}
	dst := &DstUser{}
	conv.Convert(src, dst)

	require.Equal(t, "Eve", dst.Name)
}

func TestConvert_Slices(t *testing.T) {
	type SrcItem struct {
		ID   int
		Name string
	}
	type DstItem struct {
		ID   int
		Name string
	}
	type SrcList struct {
		Items []SrcItem
	}
	type DstList struct {
		Items []DstItem
	}

	src := &SrcList{Items: []SrcItem{{ID: 1, Name: "a"}, {ID: 2, Name: "b"}}}
	dst := &DstList{}

	converter.Convert(src, dst)

	require.Len(t, dst.Items, 2)
	require.Equal(t, "a", dst.Items[0].Name)
}

func TestConvertSeq_Basic(t *testing.T) {
	src := []SrcUser{
		{Name: "A", Age: 1},
		{Name: "B", Age: 2},
	}

	count := 0
	for dst := range converter.ConvertSeq[SrcUser, DstUser](src) {
		count++
		require.NotEmpty(t, dst.Name)
	}
	require.Equal(t, 2, count)
}

func TestNewObjectPools(t *testing.T) {
	pools := converter.NewObjectPools()
	require.NotNil(t, pools)
}

func TestNewPrimitiveRegistry(t *testing.T) {
	reg := converter.NewPrimitiveRegistry()
	require.NotNil(t, reg)
}

func TestNewTypeCache(t *testing.T) {
	tc := converter.NewTypeCache()
	require.NotNil(t, tc)
}

func TestCompileTimeTypeError_Error(t *testing.T) {
	err := converter.CompileTimeTypeError{
		Source:      "src",
		Destination: 42,
		Reason:      "incompatible",
		Original:    errors.New("orig"),
	}
	msg := err.Error()
	require.NotEmpty(t, msg)
}

func TestCompileTimeTypeError_Unwrap(t *testing.T) {
	orig := errors.New("original")
	err := converter.CompileTimeTypeError{Original: orig}
	require.ErrorIs(t, err, orig)
}

func TestCompileTimeTypeError_Unwrap_NonError(t *testing.T) {
	err := converter.CompileTimeTypeError{Original: "not an error"}
	require.Nil(t, err.Unwrap())
}

func TestObjectPools_StringSlice(t *testing.T) {
	p := converter.NewObjectPools()
	s := p.GetStringSlice(5)
	require.GreaterOrEqual(t, cap(*s), 5)
	*s = append(*s, "a", "b")
	p.PutStringSlice(s)
}

func TestObjectPools_BoolSlice_PutGet(t *testing.T) {
	p := converter.NewObjectPools()
	s := p.GetBoolSlice(3)
	require.Len(t, *s, 3)
	p.PutBoolSlice(s)
}

func TestObjectPools_ValueSlice_PutGet(t *testing.T) {
	p := converter.NewObjectPools()
	s := p.GetValueSlice(4)
	require.Len(t, *s, 4)
	p.PutValueSlice(s)
}

func TestObjectPools_PutNil(t *testing.T) {
	p := converter.NewObjectPools()
	p.PutStringSlice(nil)
	p.PutBoolSlice(nil)
	p.PutValueSlice(nil)
}

func TestTypeCache_InvalidateAndClear(t *testing.T) {
	tc := converter.NewTypeCache()
	typ := reflect.TypeFor[struct{ Name string }]()
	_ = tc.GetTypeInfo(typ)
	tc.InvalidateType(typ)
	info := tc.GetTypeInfo(typ)
	require.NotNil(t, info, "GetTypeInfo after InvalidateType should not return nil")
	tc.Clear()
	info = tc.GetTypeInfo(typ)
	require.NotNil(t, info, "GetTypeInfo after Clear should not return nil")
}

func TestWithCodecs_Empty(t *testing.T) {
	conv := converter.New[*SrcUser, *DstUser](converter.WithCodecs())
	require.NotNil(t, conv)
}

// TestWithCodecs_InterceptsStructKind is a regression test: a registered codec
// must be consulted before the built-in struct-to-struct field copy. Otherwise a
// codec-handled type whose Go kind is struct is shadowed by field-by-field copy,
// which (for differing field names or unexported-only fields like time.Time)
// silently yields a zero value.
func TestWithCodecs_InterceptsStructKind(t *testing.T) {
	type legacyName struct{ Full string }
	type displayName struct{ Value string }
	type src struct {
		ID   string
		Name legacyName
	}
	type dst struct {
		ID   string
		Name displayName
	}

	legacyType := reflect.TypeOf(legacyName{})
	displayType := reflect.TypeOf(displayName{})

	// Maps legacyName -> displayName. Field names differ, so the default struct
	// copy would leave Value empty; only the codec can populate it.
	codec := func(field string, s, d reflect.Value, next convcodec.CodecHandler) {
		si := reflect.Indirect(s)
		di := reflect.Indirect(d)
		if si.IsValid() && di.IsValid() && si.Type() == legacyType && di.Type() == displayType {
			di.FieldByName("Value").SetString(si.FieldByName("Full").String())
			return
		}
		next(field, s, d)
	}

	var out dst
	converter.Convert(src{ID: "u1", Name: legacyName{Full: "Ada"}}, &out, converter.WithCodecs(codec))

	require.Equal(t, "u1", out.ID, "plain assignable field must still convert with a codec registered")
	require.Equal(t, "Ada", out.Name.Value, "codec must intercept the struct-kind field")
}

// TestWithCodecs_ConvertibleFallbackStillRuns guards the refactor that made the
// built-in dispatch the codec chain's terminal handler: a codec that always
// delegates must still let the convertible-scalar fallback run (here a defined
// type to its underlying type).
func TestWithCodecs_ConvertibleFallbackStillRuns(t *testing.T) {
	type celsius float64
	type src struct{ Temp celsius }
	type dst struct{ Temp float64 }

	// Never handles anything; always delegates to the rest of the chain.
	passthrough := func(field string, s, d reflect.Value, next convcodec.CodecHandler) {
		next(field, s, d)
	}

	var out dst
	converter.Convert(src{Temp: celsius(36.6)}, &out, converter.WithCodecs(passthrough))

	require.InEpsilon(t, 36.6, out.Temp, 1e-9, "convertible scalar fallback must run after a delegating codec")
}

// TestWithCodecs_InterceptsSliceAndMapElements widens
// TestWithCodecs_InterceptsStructKind to cover slice and map element
// types. The codec hook fires per-element during slice / map traversal
// — a regression that skipped the codec on element conversion would
// leave the synthetic struct payload empty for the whole collection,
// not just one field.
func TestWithCodecs_InterceptsSliceAndMapElements(t *testing.T) {
	type legacyName struct{ Full string }
	type displayName struct{ Value string }
	type src struct {
		Names    []legacyName
		ByRegion map[string]legacyName
	}
	type dst struct {
		Names    []displayName
		ByRegion map[string]displayName
	}

	legacyType := reflect.TypeOf(legacyName{})
	displayType := reflect.TypeOf(displayName{})

	codec := func(field string, s, d reflect.Value, next convcodec.CodecHandler) {
		si := reflect.Indirect(s)
		di := reflect.Indirect(d)
		if si.IsValid() && di.IsValid() && si.Type() == legacyType && di.Type() == displayType {
			di.FieldByName("Value").SetString(si.FieldByName("Full").String())
			return
		}
		next(field, s, d)
	}

	in := src{
		Names: []legacyName{{Full: "Ada"}, {Full: "Bob"}},
		ByRegion: map[string]legacyName{
			"eu": {Full: "Carol"},
			"us": {Full: "Dan"},
		},
	}
	var out dst
	converter.Convert(in, &out, converter.WithCodecs(codec))

	require.Len(t, out.Names, 2)
	require.Equal(t, "Ada", out.Names[0].Value)
	require.Equal(t, "Bob", out.Names[1].Value)
	require.Equal(t, "Carol", out.ByRegion["eu"].Value)
	require.Equal(t, "Dan", out.ByRegion["us"].Value)
}

// TestWithCodecs_InterceptsNestedStructField pins the codec dispatch
// for a struct field one level deep. The current implementation calls
// convertValue recursively while walking struct fields — this test
// proves the codec sees nested codec-handled fields, not just the
// outermost one. Without it a future change that broke recursion
// (e.g. an "only fire codecs at the top level" optimization) would
// silently regress.
func TestWithCodecs_InterceptsNestedStructField(t *testing.T) {
	type legacyName struct{ Full string }
	type displayName struct{ Value string }
	type srcInner struct{ Name legacyName }
	type dstInner struct{ Name displayName }
	type src struct{ Inner srcInner }
	type dst struct{ Inner dstInner }

	legacyType := reflect.TypeOf(legacyName{})
	displayType := reflect.TypeOf(displayName{})

	codec := func(field string, s, d reflect.Value, next convcodec.CodecHandler) {
		si := reflect.Indirect(s)
		di := reflect.Indirect(d)
		if si.IsValid() && di.IsValid() && si.Type() == legacyType && di.Type() == displayType {
			di.FieldByName("Value").SetString(si.FieldByName("Full").String())
			return
		}
		next(field, s, d)
	}

	var out dst
	converter.Convert(src{Inner: srcInner{Name: legacyName{Full: "Ada"}}}, &out, converter.WithCodecs(codec))
	require.Equal(t, "Ada", out.Inner.Name.Value, "codec must intercept the codec-handled struct field even when wrapped in another struct")
}

func TestConvert_EmbeddedStructs(t *testing.T) {
	type Base struct {
		ID int
	}
	type SrcObj struct {
		Base
		Val string
	}
	type DstObj struct {
		Base
		Val string
	}

	src := &SrcObj{Base: Base{ID: 1}, Val: "test"}
	dst := &DstObj{}

	converter.Convert(src, dst, converter.WithHandleEmbeddedStructs(true))

	require.Equal(t, "test", dst.Val)
}

func TestConvert_OverflowCheck_Int64ToInt8(t *testing.T) {
	type Src struct{ Age int64 }
	type Dst struct{ Age int8 }

	src := &Src{Age: 1000}
	dst := &Dst{}

	defer func() {
		r := recover()
		require.NotNil(t, r, "expected panic for int64(1000) → int8 with overflow check")
		oe, ok := r.(converter.OverflowError)
		require.True(t, ok, "expected OverflowError, got %T: %v", r, r)
		require.Equal(t, reflect.Int64, oe.From)
		require.Equal(t, reflect.Int8, oe.To)
		require.NotEmpty(t, oe.Error())
	}()

	converter.Convert(src, dst, converter.WithOverflowCheck())
}

func TestConvert_OverflowCheck_Uint64ToUint8(t *testing.T) {
	type Src struct{ Count uint64 }
	type Dst struct{ Count uint8 }

	src := &Src{Count: 300}
	dst := &Dst{}

	defer func() {
		r := recover()
		require.NotNil(t, r, "expected panic for uint64(300) → uint8 with overflow check")
		oe, ok := r.(converter.OverflowError)
		require.True(t, ok, "expected OverflowError, got %T: %v", r, r)
		require.Equal(t, reflect.Uint64, oe.From)
		require.Equal(t, reflect.Uint8, oe.To)
	}()

	converter.Convert(src, dst, converter.WithOverflowCheck())
}

func TestConvert_OverflowCheck_FitsInRange(t *testing.T) {
	type Src struct{ Age int64 }
	type Dst struct{ Age int8 }

	src := &Src{Age: 42}
	dst := &Dst{}

	converter.Convert(src, dst, converter.WithOverflowCheck())

	require.Equal(t, int8(42), dst.Age)
}

func TestConvert_OverflowCheck_Disabled(t *testing.T) {
	type Src struct{ Age int64 }
	type Dst struct{ Age int8 }

	src := &Src{Age: 1000}
	dst := &Dst{}

	// Without WithOverflowCheck — should silently truncate (backwards compat)
	converter.Convert(src, dst)

	// Value is truncated, not 1000
	require.True(t, dst.Age != 0 || src.Age == 0, "conversion should have happened even with truncation")
}
