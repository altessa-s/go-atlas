// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package ptr provides helpers for creating and dereferencing pointers to primitive types.
// Useful for struct initialization with optional fields or APIs requiring pointer values.
//
// All exported functions are pure and thread-safe.
//
// # Usage
//
//	// Create pointers to literals without intermediate variables
//	strPtr := ptr.Wrap("hello")
//	numPtr := ptr.Wrap(42)
//
//	// Create pointer only if non-zero
//	ptr.WrapNonZero(0)      // nil
//	ptr.WrapNonZero(42)     // *int pointing to 42
//
//	// Safe dereference with default values
//	ptr.Unwrap(strPtr)          // "hello"
//	ptr.Unwrap(nil, "default")  // "default"
//
//	// Struct initialization
//	cfg := Config{
//	    Timeout: ptr.Wrap(30),
//	    Name:    ptr.WrapNonZero(name), // nil if name is empty
//	}
package ptr
