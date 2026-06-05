// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package optional

import (
	"reflect"
	"strings"
	"unsafe"
)

// reflectionPackagePath is the canonical import path of this package as
// reported by reflect.Type.PkgPath.
const reflectionPackagePath = "github.com/altessa-s/go-atlas/core/types/optional"

// reflectionNamePrefix matches the unqualified name reported by
// reflect.Type.Name for any instantiation of Optional[T] (e.g.
// "Optional[time.Time]", "Optional[*github.com/.../Foo]").
const reflectionNamePrefix = "Optional["

// IsOptionalType reports whether t is an Optional[T] instantiation
// declared by this package. It returns false for nil, for non-struct
// kinds, and for look-alike types declared elsewhere.
//
// IsOptionalType is intended for codecs and serializers that need to
// detect Optional fields by reflection; ordinary user code should rely
// on the type system instead.
func IsOptionalType(t reflect.Type) bool {
	if t == nil || t.Kind() != reflect.Struct {
		return false
	}
	if t.PkgPath() != reflectionPackagePath {
		return false
	}
	return strings.HasPrefix(t.Name(), reflectionNamePrefix)
}

// InnerType returns the type parameter T of an Optional[T]. It panics
// when optType is not an Optional[T] instantiation; check with
// IsOptionalType first when the input is not statically known.
func InnerType(optType reflect.Type) reflect.Type {
	if !IsOptionalType(optType) {
		panic("optional.InnerType: not an Optional[T] type: " + describeType(optType))
	}
	return optType.Field(0).Type
}

// GetReflect extracts the (value, present) pair from an Optional[T]
// reflect.Value. The returned value is a reflect.Value of type T; when
// present is false it carries the zero value of T.
//
// GetReflect panics when opt is not a struct value passing IsOptionalType.
// If opt is not addressable it is first copied into an addressable
// temporary, so callers may pass non-addressable values (for example,
// reflect.ValueOf(someOptional) directly) without an explicit New/Set
// dance.
func GetReflect(opt reflect.Value) (reflect.Value, bool) {
	if !IsOptionalType(opt.Type()) {
		panic("optional.GetReflect: not an Optional[T] value: " + describeType(opt.Type()))
	}
	if !opt.CanAddr() {
		// Copy into an addressable temporary so callers don't have to
		// pre-allocate when they only have a value (e.g. extracted from
		// a slice element header).
		addr := reflect.New(opt.Type()).Elem()
		addr.Set(opt)
		opt = addr
	}
	present := opt.Field(1).Bool()
	value := readUnexportedField(opt.Field(0))
	return value, present
}

// SomeReflect builds Some(value) as a reflect.Value of type optType.
// optType must be an Optional[T] instantiation, and value must be
// assignable to T; both conditions panic on violation.
func SomeReflect(optType reflect.Type, value reflect.Value) reflect.Value {
	if !IsOptionalType(optType) {
		panic("optional.SomeReflect: not an Optional[T] type: " + describeType(optType))
	}
	inner := optType.Field(0).Type
	if !value.IsValid() {
		panic("optional.SomeReflect: invalid value")
	}
	if !value.Type().AssignableTo(inner) {
		panic("optional.SomeReflect: value of type " + describeType(value.Type()) +
			" not assignable to inner type " + describeType(inner))
	}
	out := reflect.New(optType).Elem()
	writeUnexportedField(out.Field(0), value)
	writeUnexportedField(out.Field(1), reflect.ValueOf(true))
	return out
}

// NoneReflect returns a None reflect.Value of type optType.
// optType must be an Optional[T] instantiation.
func NoneReflect(optType reflect.Type) reflect.Value {
	if !IsOptionalType(optType) {
		panic("optional.NoneReflect: not an Optional[T] type: " + describeType(optType))
	}
	return reflect.New(optType).Elem()
}

// readUnexportedField returns an addressable, exported reflect.Value
// aliased over field's memory. This bypasses reflect's safety check
// on unexported fields, which is sound here because the layout of
// Optional[T] is fixed and the helper lives in the same package as
// the type definition.
func readUnexportedField(field reflect.Value) reflect.Value {
	return reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem()
}

// writeUnexportedField stores src into the unexported field dst by
// aliasing dst's storage through unsafe.Pointer.
func writeUnexportedField(dst, src reflect.Value) {
	reflect.NewAt(dst.Type(), unsafe.Pointer(dst.UnsafeAddr())).Elem().Set(src)
}

// describeType renders a nil-safe type label for panic messages.
func describeType(t reflect.Type) string {
	if t == nil {
		return "<nil>"
	}
	return t.String()
}
