// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package strings provides utilities for string manipulation, conversion, and secure operations.
// Includes validation, case transformation, string interning, and SecureString for sensitive data.
//
// All utility functions are pure and thread-safe. SecureString is safe
// for concurrent reads. Interner is fully thread-safe with lock-free design.
//
// # Options Pattern
//
// Functions with multiple configuration options use Options structs:
//   - [JoinOptions] for [Join]: separator, prefix, suffix, skip empty.
//   - [SplitOptions] for [Split]: separator, max splits, trim, case sensitivity.
//   - [ContainsOptions] for [Contains]: case sensitivity, whole words, counting.
//
// # Usage
//
//	// Validation and conversion
//	strings.IsEmpty("  ")              // true
//	ptr := strings.ToPtr("hello")      // *string
//
//	// Case transformation
//	strings.ToSnakeCase("HelloWorld")  // "hello_world"
//	strings.ToCamelCase("hello_world") // "helloWorld"
//
//	// Secure string storage
//	secure := strings.NewSecureString("password")
//	defer secure.Clear() // Explicitly zero memory
//
//	// String interning (memory optimization)
//	canonical := strings.InternString("/api/users")
//
// # Performance
//
//   - Small string optimization: SecureString stores ≤64 bytes inline (no heap allocation).
//   - String interning: 20-30% memory reduction with lock-free hot cache.
//   - Zero-copy operations: ToBytesUnsafe/FromBytesUnsafe avoid allocations.
//   - Object pooling: SecureString automatically reuses instances to reduce GC pressure.
package strings
