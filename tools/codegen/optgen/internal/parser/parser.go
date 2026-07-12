// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package parser

import (
	"go/ast"
	"maps"
	"slices"
	"strings"
	"unicode"

	"github.com/altessa-s/go-atlas/tools/codegen/optgen/model"
	"github.com/altessa-s/go-atlas/tools/codegen/optgen/plugin"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// capitalizeFirst returns the string with its first letter capitalized.
// Used to derive option names from field names (e.g., "envPrefix" -> "EnvPrefix").
func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// derivePackageName extracts the package name from an import path.
// Handles special cases:
//   - Versioned imports: github.com/redis/go-redis/v9 → go-redis
//   - .go suffix: github.com/nats-io/nats.go → nats
//   - Regular imports: github.com/foo/bar → bar
//
// Note that the result is the last meaningful path segment, which may differ
// from the actual Go package name (e.g. go-redis vs package redis); callers
// that need the real package name must handle that themselves.
func derivePackageName(path string) string {
	parts := strings.Split(path, "/")
	pkgName := parts[len(parts)-1]

	// Handle versioned imports (v2, v3, ..., v9, v10, etc.)
	// If last segment is a version, use the previous segment
	if isVersionSegment(pkgName) && len(parts) >= 2 {
		pkgName = parts[len(parts)-2]
	}

	// Handle special case: import path ending with ".go" suffix
	// e.g., github.com/nats-io/nats.go -> package name is "nats"
	if trimmed, found := strings.CutSuffix(pkgName, ".go"); found {
		pkgName = trimmed
	}

	return pkgName
}

// isVersionSegment checks if a path segment is a Go module version (v2, v3, etc.)
func isVersionSegment(s string) bool {
	if len(s) < 2 || s[0] != 'v' {
		return false
	}
	for _, c := range s[1:] {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// FindOptFieldsResult contains the result of parsing opt fields from a struct.
type FindOptFieldsResult struct {
	Fields      []model.OptField            // Parsed fields with opt tags
	Imports     map[string]model.ImportInfo // Import alias to ImportInfo mapping
	GenericInfo model.GenericInfo           // Generic type parameters if the struct is generic
}

// FindOptFields finds all fields with opt tags in the specified struct type.
// It scans the AST package for the target struct and extracts field information
// from opt tags. It also extracts generic type parameters if the struct is generic.
//
// Parameters:
//   - pkg: The AST package to scan
//   - typeName: The struct type name to find
//   - processAllFields: If true, process all fields (not just those with opt tags)
//
// Returns:
//   - FindOptFieldsResult containing fields, imports, and generic info
//   - error: Validation errors or parsing errors
//
//nolint:staticcheck // SA1019: ast.Package is deprecated; optgen intentionally operates on go/ast packages.
func FindOptFields(pkg *ast.Package, typeName string, processAllFields bool) (FindOptFieldsResult, error) {
	result := FindOptFieldsResult{
		Fields:  []model.OptField{},
		Imports: make(map[string]model.ImportInfo),
	}
	var parseErr error

	fileNames := slices.Sorted(maps.Keys(pkg.Files))

	for _, name := range fileNames {
		file := pkg.Files[name]

		ast.Inspect(file, func(n ast.Node) bool {
			ts, ok := n.(*ast.TypeSpec)
			if !ok || ts.Name.Name != typeName {
				return true
			}

			// Found the target struct! Collect imports from THIS file only.
			collectFileImports(file, result.Imports)

			// Extract generic type parameters if present
			if typeParams := ExtractTypeParams(ts); len(typeParams) > 0 {
				result.GenericInfo = model.GenericInfo{TypeParams: typeParams}
			}

			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				return true
			}

			// Process each field in the struct
			for _, field := range st.Fields.List {
				optField, shouldInclude, err := processStructField(field, processAllFields)
				if err != nil {
					parseErr = err
					return false
				}
				if shouldInclude {
					result.Fields = append(result.Fields, optField)
				}
			}

			return true
		})
	}

	sortOptFields(result.Fields)

	return result, parseErr
}

// collectFileImports extracts import information from a file's imports.
// This avoids cross-file import conflicts (e.g., "time" vs "custom/time").
func collectFileImports(file *ast.File, imports map[string]model.ImportInfo) {
	for _, imp := range file.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		pkgName := derivePackageName(path)

		alias := ""
		if imp.Name != nil {
			alias = imp.Name.Name
		}

		// Determine the key for lookup in ExtractImportsFromType.
		// The key must match what's used in the type expression:
		// - For aliased imports (vaultApi "pkg/api"), key = "vaultApi"
		// - For blank imports (_ "pkg"), key = package name, alias becomes ""
		// - For regular imports ("pkg"), key = package name
		key := pkgName
		if alias != "" && alias != "_" && alias != "." {
			key = alias
		}
		if alias == "_" {
			alias = "" // Convert blank import to regular import in generated code
		}
		imports[key] = model.ImportInfo{Path: path, Alias: alias}
	}
}

// fieldProcessingContext holds intermediate state during field processing.
type fieldProcessingContext struct {
	tagStr      string
	tag         string
	hasOptTag   bool
	optgenTag   string
	hasOptgen   bool
	defaultOnly bool
}

// processStructField processes a single struct field and returns the OptField if it should be included.
// Returns (OptField, shouldInclude, error).
func processStructField(field *ast.Field, processAllFields bool) (model.OptField, bool, error) {
	ctx := extractFieldContext(field)

	// Check if field should be skipped or marked as DefaultOnly
	if ctx.hasOptTag {
		_, skip := ParseOptName(ctx.tag)
		if skip {
			// Check if there's a default value - if so, include as DefaultOnly
			if ctx.hasOptgen && hasDefaultInOptgen(ctx.optgenTag) {
				ctx.defaultOnly = true
			} else {
				return model.OptField{}, false, nil
			}
		}
	}

	// Determine if we should process this field
	if !shouldProcessField(ctx, processAllFields) {
		return model.OptField{}, false, nil
	}

	// Build and return the OptField
	return buildOptField(field, ctx)
}

// extractFieldContext extracts tag information from a field.
func extractFieldContext(field *ast.Field) fieldProcessingContext {
	tagStr := ""
	if field.Tag != nil {
		tagStr = field.Tag.Value
	}

	tag, hasOptTag := LookupTag(tagStr, "opt")
	optgenTag, hasOptgen := LookupTag(tagStr, "optgen")

	return fieldProcessingContext{
		tagStr:    tagStr,
		tag:       tag,
		hasOptTag: hasOptTag,
		optgenTag: optgenTag,
		hasOptgen: hasOptgen,
	}
}

// shouldProcessField determines if a field should be processed based on its tags.
func shouldProcessField(ctx fieldProcessingContext, processAllFields bool) bool {
	switch {
	case ctx.defaultOnly:
		return true
	case processAllFields:
		return true
	case ctx.hasOptTag:
		return true
	default:
		_, hasOptval := LookupTag(ctx.tagStr, "optval")
		_, hasOptcheck := LookupTag(ctx.tagStr, "optcheck")
		return ctx.hasOptgen || hasOptval || hasOptcheck
	}
}

// buildOptField constructs an OptField from a field and its context.
func buildOptField(field *ast.Field, ctx fieldProcessingContext) (model.OptField, bool, error) {
	optName, _ := ParseOptName(ctx.tag)

	// Get field name
	fieldName := ""
	if len(field.Names) > 0 {
		fieldName = field.Names[0].Name
	}

	// If optName is empty, derive from field name (capitalize first letter)
	if optName == "" && fieldName != "" {
		optName = capitalizeFirst(fieldName)
	}

	parsed, err := parseFieldTags(ctx.tagStr)
	if err != nil {
		return model.OptField{}, false, coreerrs.Wrapf(err, "failed to parse tags for field %s", fieldName)
	}

	// Extract type information
	typeStr := TypeToString(field.Type)
	arrType, isSlice := field.Type.(*ast.ArrayType)
	elemType := ""
	if isSlice && arrType != nil {
		elemType = TypeToString(arrType.Elt)
	}

	// Extract documentation
	doc := extractFieldDoc(field)

	// Get default value from plugins if not explicitly set
	if parsed.Default == "" {
		parsed.Default = plugin.FindTypeDefaultPlugin(typeStr)
	}

	// Get and merge modifiers
	modifiers := getFieldModifiers(typeStr, elemType, isSlice, parsed.Modifiers)

	return model.OptField{
		FieldName:    fieldName,
		OptionName:   optName,
		Type:         typeStr,
		TypeExpr:     field.Type,
		Default:      parsed.Default,
		Doc:          doc,
		IsSlice:      isSlice,
		IsInterface:  IsInterfaceType(field.Type),
		IsNilable:    IsNilableType(field.Type),
		ElemType:     elemType,
		Modifiers:    modifiers,
		Metadata:     parsed.Metadata,
		Checks:       parsed.Checks,
		HasModifiers: len(modifiers) > 0,
		DefaultOnly:  ctx.defaultOnly,
	}, true, nil
}

// extractFieldDoc extracts documentation from a field's Doc or Comment.
func extractFieldDoc(field *ast.Field) string {
	if field.Doc != nil {
		return strings.TrimSpace(field.Doc.Text())
	}
	if field.Comment != nil {
		return strings.TrimSpace(field.Comment.Text())
	}
	return ""
}

// getFieldModifiers gets and merges modifiers for a field type.
func getFieldModifiers(typeStr, elemType string, isSlice bool, explicitMods []string) []string {
	var defaultMods []string

	if isSlice && elemType != "" {
		// Element-level defaults (e.g., trimspaces for []string)
		defaultMods = plugin.GetDefaultModifiersForType(elemType, explicitMods)
		// Slice-level defaults (e.g., dedup for any [])
		sliceMods := plugin.GetDefaultModifiersForType(typeStr, explicitMods)
		defaultMods = append(defaultMods, sliceMods...)
	} else {
		defaultMods = plugin.GetDefaultModifiersForType(typeStr, explicitMods)
	}

	// Merge: defaults first, then explicit modifiers (excluding disablers)
	modifiers := make([]string, 0, len(defaultMods)+len(explicitMods))
	modifiers = append(modifiers, defaultMods...)
	modifiers = append(modifiers, explicitMods...)

	return removeDisablers(modifiers)
}

// sortOptFields sorts fields by OptionName, then FieldName.
func sortOptFields(fields []model.OptField) {
	slices.SortFunc(fields, func(a, b model.OptField) int {
		if a.OptionName != b.OptionName {
			return strings.Compare(a.OptionName, b.OptionName)
		}
		return strings.Compare(a.FieldName, b.FieldName)
	})
}

// removeDisablers removes disabler modifiers from the list.
// Disablers are special modifiers that only prevent default modifiers from being applied
// (e.g., "notrim" disables automatic "trimspaces").
func removeDisablers(modifiers []string) []string {
	disablers := plugin.GetAllDisablers()
	if len(disablers) == 0 {
		return modifiers
	}

	disablerSet := make(map[string]bool, len(disablers))
	for _, d := range disablers {
		disablerSet[d] = true
	}

	result := make([]string, 0, len(modifiers))
	for _, m := range modifiers {
		if !disablerSet[m] {
			result = append(result, m)
		}
	}
	return result
}

// hasDefaultInOptgen checks if the optgen tag contains a default value specification.
func hasDefaultInOptgen(optgenTag string) bool {
	return strings.Contains(optgenTag, "default=")
}

// FindNormalizationMethod searches for a Normalization or normalization method
// on the specified struct type. Returns the method name if found, or empty string otherwise.
//
//nolint:staticcheck // SA1019: ast.Package is deprecated; optgen intentionally operates on go/ast packages.
func FindNormalizationMethod(pkg *ast.Package, typeName string) string {
	for _, file := range pkg.Files {
		for _, decl := range file.Decls {
			funcDecl, ok := decl.(*ast.FuncDecl)
			if !ok || funcDecl.Recv == nil || len(funcDecl.Recv.List) == 0 {
				continue
			}

			// Check if this is a method on our type (pointer or value receiver)
			recv := funcDecl.Recv.List[0].Type
			var recvTypeName string
			switch t := recv.(type) {
			case *ast.StarExpr:
				if ident, ok := t.X.(*ast.Ident); ok {
					recvTypeName = ident.Name
				}
			case *ast.Ident:
				recvTypeName = t.Name
			}

			if recvTypeName != typeName {
				continue
			}

			// Check for Normalization or normalization method
			methodName := funcDecl.Name.Name
			if methodName == "Normalization" || methodName == "normalization" {
				return methodName
			}
		}
	}
	return ""
}
