// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package normalizer provides tag-based string normalization for struct fields.
//
// Struct fields annotated with `normalize:"..."` tags are processed by
// [Normalize]. Multiple modifiers can be chained with commas
// (e.g. "trim,lowercase"), and modifiers may accept parameters in
// parentheses (e.g. "phone(region=US)"). See the [modifiers] sub-package
// for built-in modifiers (trim, lowercase, uppercase, nil_on_empty, phone,
// remove_bad_symbols, remove_empty_elements).
//
// Special tag values:
//   - "-" skips the field entirely
//   - "custom" skips built-in modifiers but still invokes [CustomNormalizer]
//
// Nested structs, slices, and arrays are traversed recursively. Struct types
// that implement [CustomNormalizer] receive a final Normalize() call after all
// tag-based modifiers have been applied.
//
// Example:
//
//	type User struct {
//	    Name  string  `normalize:"trim,lowercase"`
//	    Email *string `normalize:"trim,nil_on_empty"`
//	    Phone string  `normalize:"phone(region=US)"`
//	}
//	user := &User{Name: "  John Doe  ", Phone: "(212) 555-1234"}
//	normalizer.Normalize(user)
package normalizer
