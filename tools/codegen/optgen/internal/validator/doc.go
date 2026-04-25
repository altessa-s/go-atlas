// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package validator provides compile-safety checks for Go identifiers used in
// generated code. It is called by the optgen pipeline before code generation to
// ensure that user-supplied type names and option names produce valid Go source.
//
// # Usage
//
//	if err := validator.ValidateGoIdentifier("MyType"); err != nil {
//	    // name is not a legal Go identifier
//	}
package validator
