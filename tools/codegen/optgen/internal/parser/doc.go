// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package parser extracts struct field metadata from Go AST packages for the
// optgen code generator.
//
// The primary entry point is [FindOptFields], which locates a named struct type,
// walks its fields, and returns a [FindOptFieldsResult] containing parsed
// [model.OptField] values, resolved imports, and generic type parameter
// information. Tag parsing covers four tag keys:
//
//   - opt -- controls option naming and skip behavior
//   - optgen -- controls defaults, metadata flags, and manual mode
//   - optval -- lists value modifiers (transforms, post-processors, guards)
//   - optcheck -- lists validation checks (required, minlen, maxlen, oneof, nonzero)
//
// Helper functions convert AST type expressions to strings ([TypeToString]),
// extract imports referenced by a type ([ExtractImportsFromType]), and parse
// individual tag grammars.
//
// # Usage
//
//	result, err := parser.FindOptFields(pkg, "Options", false)
package parser
