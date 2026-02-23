// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package converter_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/altessa-s/go-atlas/domain/converter"
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

	if dst.Name != "Alice" {
		t.Errorf("Name = %q, want Alice", dst.Name)
	}
	if dst.Email != "a@b.com" {
		t.Errorf("Email = %q", dst.Email)
	}
	if dst.Age != 30 {
		t.Errorf("Age = %d, want 30", dst.Age)
	}
}

func TestConvert_IgnoreZeroValues(t *testing.T) {
	src := &SrcUser{Name: "Bob", Age: 0}
	dst := &DstUser{Name: "Original", Age: 25}

	converter.Convert(src, dst, converter.WithIgnoreZeroValues())

	if dst.Name != "Bob" {
		t.Errorf("Name = %q, want Bob", dst.Name)
	}
	if dst.Age != 25 {
		t.Errorf("Age = %d, want 25 (zero should be ignored)", dst.Age)
	}
}

func TestConvert_IgnoreFields(t *testing.T) {
	src := &SrcUser{Name: "Carol", Email: "c@d.com", Age: 40}
	dst := &DstUser{}

	converter.Convert(src, dst, converter.WithIgnoreFields("Email"))

	if dst.Name != "Carol" {
		t.Errorf("Name = %q", dst.Name)
	}
	if dst.Email != "" {
		t.Errorf("Email = %q, should be ignored", dst.Email)
	}
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

	if dst.Name != "Dave" {
		t.Errorf("Name = %q, want Dave", dst.Name)
	}
}

func TestNew_Converter(t *testing.T) {
	conv := converter.New[*SrcUser, *DstUser]()
	if conv == nil {
		t.Fatal("New() returned nil")
	}

	src := &SrcUser{Name: "Eve", Age: 20}
	dst := &DstUser{}
	conv.Convert(src, dst)

	if dst.Name != "Eve" {
		t.Errorf("Name = %q", dst.Name)
	}
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

	if len(dst.Items) != 2 {
		t.Fatalf("Items len = %d, want 2", len(dst.Items))
	}
	if dst.Items[0].Name != "a" {
		t.Errorf("Items[0].Name = %q", dst.Items[0].Name)
	}
}

func TestConvertSeq_Basic(t *testing.T) {
	src := []SrcUser{
		{Name: "A", Age: 1},
		{Name: "B", Age: 2},
	}

	count := 0
	for dst := range converter.ConvertSeq[SrcUser, DstUser](src) {
		count++
		if dst.Name == "" {
			t.Error("empty name in converted item")
		}
	}
	if count != 2 {
		t.Errorf("ConvertSeq yielded %d items, want 2", count)
	}
}

func TestNewObjectPools(t *testing.T) {
	pools := converter.NewObjectPools()
	if pools == nil {
		t.Fatal("NewObjectPools() returned nil")
	}
}

func TestNewPrimitiveRegistry(t *testing.T) {
	reg := converter.NewPrimitiveRegistry()
	if reg == nil {
		t.Fatal("NewPrimitiveRegistry() returned nil")
	}
}

func TestNewTypeCache(t *testing.T) {
	tc := converter.NewTypeCache()
	if tc == nil {
		t.Fatal("NewTypeCache() returned nil")
	}
}

func TestCompileTimeTypeError_Error(t *testing.T) {
	err := converter.CompileTimeTypeError{
		Source:      "src",
		Destination: 42,
		Reason:      "incompatible",
		Original:    errors.New("orig"),
	}
	msg := err.Error()
	if msg == "" {
		t.Error("Error() should not be empty")
	}
}

func TestCompileTimeTypeError_Unwrap(t *testing.T) {
	orig := errors.New("original")
	err := converter.CompileTimeTypeError{Original: orig}
	if !errors.Is(err, orig) {
		t.Error("Unwrap should return original error")
	}
}

func TestCompileTimeTypeError_Unwrap_NonError(t *testing.T) {
	err := converter.CompileTimeTypeError{Original: "not an error"}
	if err.Unwrap() != nil {
		t.Error("Unwrap should return nil for non-error original")
	}
}

func TestObjectPools_StringSlice(t *testing.T) {
	p := converter.NewObjectPools()
	s := p.GetStringSlice(5)
	if cap(s) < 5 {
		t.Errorf("GetStringSlice cap = %d, want >= 5", cap(s))
	}
	s = append(s, "a", "b")
	p.PutStringSlice(s)
}

func TestObjectPools_BoolSlice_PutGet(t *testing.T) {
	p := converter.NewObjectPools()
	s := p.GetBoolSlice(3)
	if len(s) != 3 {
		t.Errorf("GetBoolSlice length = %d, want 3", len(s))
	}
	p.PutBoolSlice(s)
}

func TestObjectPools_ValueSlice_PutGet(t *testing.T) {
	p := converter.NewObjectPools()
	s := p.GetValueSlice(4)
	if len(s) != 4 {
		t.Errorf("GetValueSlice length = %d, want 4", len(s))
	}
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
	typ := reflect.TypeOf(struct{ Name string }{})
	_ = tc.GetTypeInfo(typ)
	tc.InvalidateType(typ)
	info := tc.GetTypeInfo(typ)
	if info == nil {
		t.Fatal("GetTypeInfo after InvalidateType should not return nil")
	}
	tc.Clear()
	info = tc.GetTypeInfo(typ)
	if info == nil {
		t.Fatal("GetTypeInfo after Clear should not return nil")
	}
}

func TestWithCodecs_Empty(t *testing.T) {
	conv := converter.New[*SrcUser, *DstUser](converter.WithCodecs())
	if conv == nil {
		t.Fatal("New with WithCodecs returned nil")
	}
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

	if dst.Val != "test" {
		t.Errorf("Val = %q", dst.Val)
	}
}

func TestConvert_OverflowCheck_Int64ToInt8(t *testing.T) {
	type Src struct{ Age int64 }
	type Dst struct{ Age int8 }

	src := &Src{Age: 1000}
	dst := &Dst{}

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for int64(1000) → int8 with overflow check")
		}
		oe, ok := r.(converter.OverflowError)
		if !ok {
			t.Fatalf("expected OverflowError, got %T: %v", r, r)
		}
		if oe.From != reflect.Int64 || oe.To != reflect.Int8 {
			t.Errorf("OverflowError From=%v To=%v, want Int64→Int8", oe.From, oe.To)
		}
		if oe.Error() == "" {
			t.Error("OverflowError.Error() should not be empty")
		}
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
		if r == nil {
			t.Fatal("expected panic for uint64(300) → uint8 with overflow check")
		}
		oe, ok := r.(converter.OverflowError)
		if !ok {
			t.Fatalf("expected OverflowError, got %T: %v", r, r)
		}
		if oe.From != reflect.Uint64 || oe.To != reflect.Uint8 {
			t.Errorf("OverflowError From=%v To=%v, want Uint64→Uint8", oe.From, oe.To)
		}
	}()

	converter.Convert(src, dst, converter.WithOverflowCheck())
}

func TestConvert_OverflowCheck_FitsInRange(t *testing.T) {
	type Src struct{ Age int64 }
	type Dst struct{ Age int8 }

	src := &Src{Age: 42}
	dst := &Dst{}

	converter.Convert(src, dst, converter.WithOverflowCheck())

	if dst.Age != 42 {
		t.Errorf("Age = %d, want 42", dst.Age)
	}
}

func TestConvert_OverflowCheck_Disabled(t *testing.T) {
	type Src struct{ Age int64 }
	type Dst struct{ Age int8 }

	src := &Src{Age: 1000}
	dst := &Dst{}

	// Without WithOverflowCheck — should silently truncate (backwards compat)
	converter.Convert(src, dst)

	// Value is truncated, not 1000
	if dst.Age == 0 && src.Age != 0 {
		t.Error("conversion should have happened even with truncation")
	}
}
