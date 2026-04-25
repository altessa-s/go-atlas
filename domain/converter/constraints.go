// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package converter

// ConversionSource represents source types for conversion.
//
// Note: Go cannot currently express "any struct / pointer-to-struct / slice-of-struct" as a type set
// without making the API unusable. We intentionally keep these constraints broad and enforce the
// supported shapes at runtime (the converter panics on unsupported combinations).
//
// Example:
//
//	func Convert[T ConversionSource](src T) { ... }
type ConversionSource interface {
	any
}

// ConversionDestination represents destination types for conversion.
//
// Destinations should typically be pointers for modification. Supported shapes are validated at runtime.
//
// Example:
//
//	func Convert[T ConversionSource, U ConversionDestination](src T, dst U) { ... }
type ConversionDestination interface {
	any
}
