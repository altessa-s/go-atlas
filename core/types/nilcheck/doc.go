// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package nilcheck provides utilities for checking nil interface values.
// Handles the Go gotcha where an interface can be non-nil but contain a nil pointer.
//
// Example:
//
//	var p *MyStruct = nil
//	var i any = p
//	i != nil           // true (interface is not nil)
//	nilcheck.IsNil(i)  // true (underlying value is nil)
package nilcheck
