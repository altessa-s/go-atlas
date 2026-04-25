// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package reflect

import "reflect"

// IndirectType returns the underlying type after dereferencing all pointer levels.
// If the type is not a pointer, it returns the type unchanged.
//
// Example:
//
//	t := IndirectType(reflect.TypeOf(&User{})) // returns reflect.Type of User
func IndirectType(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}

// MakeDst ensures the destination reflect.Value is properly initialized for assignment.
// If the destination is a pointer type, it creates a new instance and redirects the Value
// to point to the newly created object's element. This is necessary for proper assignment
// when converting to pointer types.
func MakeDst(dst *reflect.Value, t reflect.Type) {
	if dst.Kind() == reflect.Pointer {
		dst.Set(reflect.New(t))
		*dst = dst.Elem()
	}
}

// IsPrimitive checks if a kind represents a primitive type.
// Primitive types include all integers, unsigned integers, floats, bool, and string.
func IsPrimitive(k reflect.Kind) bool {
	switch k {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64, reflect.Bool, reflect.String:
		return true
	default:
		return false
	}
}
