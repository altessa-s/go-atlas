// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package document

import "reflect"

// indirectType returns the underlying type by recursively dereferencing pointers.
// This function is useful when working with reflection to get the actual type
// regardless of pointer indirection levels.
//
// Parameters:
//   - t: A reflect.Type that may be a pointer or value type
//
// Returns:
//   - reflect.Type: The underlying non-pointer type
//
// Examples:
//   - *User → User
//   - **User → User
//   - User → User
//
// Use case: Type introspection when you need the base type for comparison or
// instantiation, regardless of how many levels of pointer indirection exist.
func indirectType(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}
