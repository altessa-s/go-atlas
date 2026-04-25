// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package model

import (
	"go/ast"
	"strings"
)

// OptField represents a single struct field annotated with opt/optgen/optval/optcheck tags.
// It is produced by the parser and consumed by [plugin.FieldPlugin] implementations to
// generate the corresponding WithXxx option function.
type OptField struct {
	FieldName    string            // Original field name in the struct
	OptionName   string            // Name for the WithXxx function
	Type         string            // Field type as string representation
	TypeExpr     ast.Expr          // Original AST expression for the type
	Default      string            // Default value expression
	Doc          string            // Documentation comment from field
	ElemType     string            // Element type for slices (without [])
	Modifiers    []string          // Modifier names (e.g. ["upper"], ["trimspaces", "upper"])
	Metadata     map[string]string // Additional metadata from tag (arbitrary key=value pairs)
	Checks       map[string]string // Validation rules for option values (arbitrary key=value pairs)
	IsSlice      bool              // Whether the field is a slice type
	IsInterface  bool              // Whether the field is an interface type
	IsNilable    bool              // Whether the field type can be nil (pointer, slice, map, interface, chan, func)
	HasModifiers bool              // Whether the field has any modifiers
	DefaultOnly  bool              // If true, include in defaultOptions() but skip WithXxx generation
}

// HasModifier reports whether modifier appears in the field's optval tag list.
// It short-circuits to false when HasModifiers is false.
func (f OptField) HasModifier(modifier string) bool {
	if !f.HasModifiers {
		return false
	}
	for _, mod := range f.Modifiers {
		if mod == modifier {
			return true
		}
	}
	return false
}

// ImportInfo holds a resolved package import as found in the source file being parsed.
type ImportInfo struct {
	Path  string // Import path (e.g., "log/slog")
	Alias string // Import alias (e.g., "slog")
}

// GeneratedImport represents an import to be rendered in the generated output file.
// Plugins add these via [plugin.GenerationContext.AddImport].
type GeneratedImport struct {
	Path  string // Import path (e.g., "log/slog")
	Alias string // Import alias if different from package name
}

// TypeParam represents a single type parameter from a generic type definition.
// Example: for "options[T any, U comparable]", TypeParams would be:
//   - {Name: "T", Constraint: "any"}
//   - {Name: "U", Constraint: "comparable"}
type TypeParam struct {
	Name       string // Type parameter name, e.g., "T"
	Constraint string // Type constraint, e.g., "any", "comparable", "io.Reader"
}

// GenericInfo holds generic type parameter information extracted from a struct
// definition. When the struct is non-generic, TypeParams is empty and all
// rendering methods return empty strings.
type GenericInfo struct {
	TypeParams []TypeParam // Ordered list of type parameters
}

// IsGeneric returns true if the struct has type parameters.
func (g GenericInfo) IsGeneric() bool {
	return len(g.TypeParams) > 0
}

// TypeParamsDecl returns the type parameters declaration string with constraints.
// Example: "[T any, U comparable]" or "" if not generic.
func (g GenericInfo) TypeParamsDecl() string {
	if !g.IsGeneric() {
		return ""
	}
	var b strings.Builder
	b.WriteByte('[')
	for i, tp := range g.TypeParams {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(tp.Name)
		b.WriteByte(' ')
		b.WriteString(tp.Constraint)
	}
	b.WriteByte(']')
	return b.String()
}

// TypeParamsNames returns just the type parameter names for instantiation.
// Example: "[T, U]" or "" if not generic.
func (g GenericInfo) TypeParamsNames() string {
	if !g.IsGeneric() {
		return ""
	}
	var b strings.Builder
	b.WriteByte('[')
	for i, tp := range g.TypeParams {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(tp.Name)
	}
	b.WriteByte(']')
	return b.String()
}
