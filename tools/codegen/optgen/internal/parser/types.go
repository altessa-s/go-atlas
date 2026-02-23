// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package parser

import (
	"go/ast"
	"strings"

	"github.com/altessa-s/go-atlas/tools/codegen/optgen/model"
)

// typeAny is the string representation of the any type constraint.
const typeAny = "any"

// TypeToString converts an AST type expression to a string representation.
// It handles various Go type constructs including:
//   - Basic identifiers (int, string)
//   - Qualified types (pkg.Type)
//   - Pointer types (*T)
//   - Slice types ([]T)
//   - Map types (map[K]V)
//   - Channel types (chan T, chan<- T, <-chan T)
//   - Interface types (any)
//   - Struct types (struct{})
//   - Function types (simplified as func(...))
//   - Variadic types (...T)
//   - Parenthesized expressions
func TypeToString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return TypeToString(t.X) + "." + t.Sel.Name
	case *ast.StarExpr:
		return "*" + TypeToString(t.X)
	case *ast.ArrayType:
		return "[]" + TypeToString(t.Elt)
	case *ast.MapType:
		return "map[" + TypeToString(t.Key) + "]" + TypeToString(t.Value)
	case *ast.InterfaceType:
		return typeAny
	case *ast.FuncType:
		return "func(...)" // simplified
	case *ast.ChanType:
		// Handle channel types: chan, chan<-, <-chan
		if t.Dir == ast.SEND {
			return "chan<- " + TypeToString(t.Value)
		} else if t.Dir == ast.RECV {
			return "<-chan " + TypeToString(t.Value)
		}
		return "chan " + TypeToString(t.Value)
	case *ast.StructType:
		// Handle anonymous struct types
		return "struct{}"
	case *ast.Ellipsis:
		// Handle variadic types: ...T
		return "..." + TypeToString(t.Elt)
	case *ast.ParenExpr:
		// Handle parenthesized expressions
		return TypeToString(t.X)
	case *ast.IndexExpr:
		// Handle generic type with single type argument: Container[T]
		return TypeToString(t.X) + "[" + TypeToString(t.Index) + "]"
	case *ast.IndexListExpr:
		// Handle generic type with multiple type arguments: Map[K, V]
		args := make([]string, len(t.Indices))
		for i, idx := range t.Indices {
			args[i] = TypeToString(idx)
		}
		return TypeToString(t.X) + "[" + strings.Join(args, ", ") + "]"
	default:
		return typeAny
	}
}

// ExtractImportsFromType recursively extracts package imports from a type expression.
// It walks the AST tree and finds all SelectorExpr nodes (pkg.Type patterns),
// then looks up their import paths in the provided imports map.
//
// Returns a list of import paths needed for this type.
func ExtractImportsFromType(expr ast.Expr, imports map[string]model.ImportInfo) []string {
	var pkgs []string
	seen := make(map[string]bool)

	// Get the string representation to check for standard library types
	typeStr := TypeToString(expr)

	// Handle standard library types that might not be in imports map yet
	if IsStdLibType(typeStr) {
		// Map standard library types to their import paths
		stdLibImports := map[string]string{
			"context.Context": "context",
		}

		// Remove pointer/slice prefixes for lookup
		cleanType := typeStr
		for len(cleanType) > 0 && (cleanType[0] == '*' || cleanType[0] == '[' || cleanType[0] == ']') {
			cleanType = cleanType[1:]
		}

		if importPath, exists := stdLibImports[cleanType]; exists {
			seen[importPath] = true
			pkgs = append(pkgs, importPath)
		}
	}

	ast.Inspect(expr, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			if ident, ok := sel.X.(*ast.Ident); ok {
				imp, found := imports[ident.Name]
				if !found {
					// Fallback: the actual Go package name might differ from the path segment.
					// For example, github.com/redis/go-redis/v9 has package name "redis".
					// Search all imports for a path containing the identifier as a segment.
					imp, found = findImportByPackageName(ident.Name, imports)
				}

				if found {
					key := imp.Path

					// Preserve explicit import aliases used in the type expression.
					// Example: mongoOptions.ClientOptions must import as:
					//   mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
					//
					// Use derivePackageName to correctly handle versioned imports:
					// e.g., github.com/redis/go-redis/v9 -> redis
					pkgName := derivePackageName(imp.Path)
					if ident.Name != "" && ident.Name != pkgName && ident.Name != "_" && ident.Name != "." {
						key = ident.Name + ":" + imp.Path
					}

					if !seen[key] {
						seen[key] = true
						pkgs = append(pkgs, key)
					}
				}
			}
		}
		return true
	})

	return pkgs
}

// findImportByPackageName searches for an import whose path contains the given package name.
// This handles cases where the actual Go package name differs from the path segment.
// For example, github.com/redis/go-redis/v9 has package name "redis".
func findImportByPackageName(pkgName string, imports map[string]model.ImportInfo) (model.ImportInfo, bool) {
	for _, imp := range imports {
		// Check if the package name appears as a path segment
		// e.g., for "redis" check if path contains "/redis/" or ends with "/redis"
		segments := strings.Split(imp.Path, "/")
		for _, seg := range segments {
			// Skip version segments
			if isVersionSegment(seg) {
				continue
			}
			// Handle .go suffix
			cleanSeg := seg
			if trimmed, found := strings.CutSuffix(seg, ".go"); found {
				cleanSeg = trimmed
			}
			if cleanSeg == pkgName {
				return imp, true
			}
		}
	}
	return model.ImportInfo{}, false
}

// IsStdLibType checks if a type is from the Go standard library.
// These types can be safely used in generated code by adding the appropriate import.
//
// Examples:
//   - "context.Context" -> true
//   - "time.Duration" -> true
//   - "slog.Logger" -> false (not standard library check, but common)
func IsStdLibType(typeStr string) bool {
	// Remove pointer/slice prefixes
	cleanType := typeStr
	for len(cleanType) > 0 && (cleanType[0] == '*' || cleanType[0] == '[' || cleanType[0] == ']') {
		cleanType = cleanType[1:]
	}

	// List of standard library types we want to support
	stdLibTypes := map[string]bool{
		"context.Context": true,
	}

	return stdLibTypes[cleanType]
}

// IsInterfaceType checks if the AST expression represents an interface type.
// Returns true for:
//   - any (empty interface)
//   - Named interface types (identified by convention - ends with "er" suffix or known patterns)
func IsInterfaceType(expr ast.Expr) bool {
	switch expr.(type) {
	case *ast.InterfaceType:
		return true
	default:
		return false
	}
}

// IsNilableType checks if the AST expression represents a type that can be nil.
// Returns true for pointers, slices, maps, channels, functions, and interfaces.
func IsNilableType(expr ast.Expr) bool {
	switch expr.(type) {
	case *ast.StarExpr: // pointer
		return true
	case *ast.ArrayType: // slice (arrays are handled differently but []T is nilable)
		return true
	case *ast.MapType: // map
		return true
	case *ast.ChanType: // channel
		return true
	case *ast.FuncType: // function
		return true
	case *ast.InterfaceType: // any
		return true
	default:
		return false
	}
}

// ExtractTypeParams extracts type parameters from a TypeSpec.
// Returns nil if the type is not generic.
//
// Example: for "options[T any, U comparable]", returns:
//   - []TypeParam{{Name: "T", Constraint: "any"}, {Name: "U", Constraint: "comparable"}}
func ExtractTypeParams(ts *ast.TypeSpec) []model.TypeParam {
	if ts == nil || ts.TypeParams == nil || len(ts.TypeParams.List) == 0 {
		return nil
	}

	var params []model.TypeParam
	for _, field := range ts.TypeParams.List {
		constraint := typeAny // default constraint
		if field.Type != nil {
			constraint = TypeToString(field.Type)
		}

		for _, name := range field.Names {
			params = append(params, model.TypeParam{
				Name:       name.Name,
				Constraint: constraint,
			})
		}
	}
	return params
}
