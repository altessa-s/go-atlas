// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package internal provides the core implementation for the optgen code generator.
//
// The implementation is organized into cooperating subsystems:
//
//   - [generator] -- renders Go source for functional-option functions
//   - [parser] -- extracts struct fields and opt/optgen/optval/optcheck tags from Go AST
//   - [validator] -- validates Go identifiers used in generated code
//   - [plugin/builtin] -- ships all built-in field handlers, transforms, checks,
//     guards, post-processors, and type-default providers
//
// Most callers should use the public API in the parent optgen package rather
// than importing these packages directly.
package internal
