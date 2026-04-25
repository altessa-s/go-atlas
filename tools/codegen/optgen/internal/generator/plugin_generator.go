// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package generator

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/altessa-s/go-atlas/tools/codegen/optgen/internal/parser"
	"github.com/altessa-s/go-atlas/tools/codegen/optgen/model"
	"github.com/altessa-s/go-atlas/tools/codegen/optgen/plugin"

	// Import builtin plugins to register them
	_ "github.com/altessa-s/go-atlas/tools/codegen/optgen/internal/plugin/builtin"
	_ "github.com/altessa-s/go-atlas/tools/codegen/optgen/internal/plugin/builtin/fields"
)

// GenerateInput holds every parameter needed by [Generator.Generate] to emit
// a complete Go source file. Fields that begin with "Generate" control which
// top-level declarations are included in the output.
type GenerateInput struct {
	PackageName         string
	TypeName            string
	OptionType          string
	GenerateOptionType  bool
	GenerateDefaultFunc bool
	GenerateNewFunc     bool
	OptionReturnsError  bool
	Fields              []model.OptField
	Imports             map[string]model.ImportInfo
	DefaultFuncName     string
	NewFuncName         string
	NormalizationMethod string
	GenericInfo         model.GenericInfo
}

// Generate produces a gofmt-formatted Go source file containing functional-option
// functions for every field in in.Fields. Each field is dispatched to the
// highest-priority [plugin.FieldPlugin] that can handle it; if no plugin matches,
// an error is returned. The output is deterministically ordered by option name.
func (g *Generator) Generate(in GenerateInput) ([]byte, error) {
	ctx := plugin.NewGenerationContext(in.PackageName, in.TypeName, in.OptionType, in.OptionReturnsError, in.Fields, in.Imports, in.GenericInfo)

	type funcOut struct {
		fieldName  string
		optionName string
		code       string
	}
	functionsOut := make([]funcOut, 0, len(in.Fields))
	importMap := make(map[string]model.GeneratedImport)
	var typeDefs []string
	var helpers []string
	needsTypeImports := make(map[string]bool, len(in.Fields))

	addImport := func(imp model.GeneratedImport) {
		if imp.Path == "" {
			return
		}
		if existing, ok := importMap[imp.Path]; ok && existing.Alias != "" && imp.Alias == "" {
			return
		}
		importMap[imp.Path] = imp
	}

	for _, field := range in.Fields {
		p, err := plugin.FindPlugin(field)
		if err != nil {
			return nil, fmt.Errorf("no plugin found for field %s: %w", field.FieldName, err)
		}

		result, err := p.Generate(ctx, field)
		if err != nil {
			return nil, fmt.Errorf("plugin %s failed to generate code for field %s: %w", plugin.GetPluginName(p), field.FieldName, err)
		}

		functionsOut = append(functionsOut, funcOut{
			fieldName:  field.FieldName,
			optionName: field.OptionName,
			code:       result.Code,
		})
		needsTypeImports[field.FieldName] = strings.TrimSpace(result.Code) != ""
		typeDefs = append(typeDefs, result.TypeDefs...)
		helpers = append(helpers, result.Helpers...)
	}

	for _, imp := range ctx.GetAdditionalImports() {
		addImport(imp)
	}

	requiredImports := g.collectImports(in.Fields, in.Imports, needsTypeImports)
	for _, imp := range requiredImports {
		addImport(imp)
	}

	finalImports := make([]model.GeneratedImport, 0, len(importMap))
	for _, imp := range importMap {
		finalImports = append(finalImports, imp)
	}
	slices.SortFunc(finalImports, func(a, b model.GeneratedImport) int {
		if c := cmp.Compare(a.Path, b.Path); c != 0 {
			return c
		}
		return cmp.Compare(a.Alias, b.Alias)
	})

	slices.Sort(typeDefs)
	slices.Sort(helpers)
	slices.SortFunc(functionsOut, func(a, b funcOut) int {
		if c := cmp.Compare(a.optionName, b.optionName); c != 0 {
			return c
		}
		return cmp.Compare(a.fieldName, b.fieldName)
	})
	generatedFunctions := make([]string, 0, len(functionsOut))
	for _, fn := range functionsOut {
		generatedFunctions = append(generatedFunctions, fn.code)
	}

	data := struct {
		Package             string
		TypeName            string
		OptionType          string
		GenerateOptionType  bool
		GenerateDefaultFunc bool
		GenerateNewFunc     bool
		OptionReturnsError  bool
		DefaultFuncName     string
		NewFuncName         string
		NormalizationMethod string
		Imports             []model.GeneratedImport
		TypeDefs            []string
		Functions           []string
		Helpers             []string
		Fields              []model.OptField
		GenericInfo         model.GenericInfo
	}{
		Package:             in.PackageName,
		TypeName:            in.TypeName,
		OptionType:          in.OptionType,
		GenerateOptionType:  in.GenerateOptionType,
		GenerateDefaultFunc: in.GenerateDefaultFunc,
		GenerateNewFunc:     in.GenerateNewFunc,
		OptionReturnsError:  in.OptionReturnsError,
		DefaultFuncName:     in.DefaultFuncName,
		NewFuncName:         in.NewFuncName,
		NormalizationMethod: in.NormalizationMethod,
		Imports:             finalImports,
		TypeDefs:            typeDefs,
		Functions:           generatedFunctions,
		Helpers:             helpers,
		Fields:              in.Fields,
		GenericInfo:         in.GenericInfo,
	}

	return g.renderPluginTemplate(data)
}

// extractPackagePrefixes extracts package prefixes from a Go expression.
// For "base64.NewKeyDecoder()" returns ["base64"].
// For "json.NewValueDecoder[T]()" returns ["json"].
func extractPackagePrefixes(expr string) []string {
	var prefixes []string
	// Simple regex-like matching: find "identifier." patterns
	// This handles cases like "pkg.Func()" or "pkg.Type{}"
	i := 0
	for i < len(expr) {
		// Skip non-identifier characters
		for i < len(expr) && !isIdentStart(expr[i]) {
			i++
		}
		if i >= len(expr) {
			break
		}
		// Collect identifier
		start := i
		for i < len(expr) && isIdentContinue(expr[i]) {
			i++
		}
		ident := expr[start:i]
		// Check if followed by '.'
		if i < len(expr) && expr[i] == '.' {
			// Check next char is also identifier (not "...")
			if i+1 < len(expr) && isIdentStart(expr[i+1]) {
				prefixes = append(prefixes, ident)
			}
		}
	}
	return prefixes
}

func isIdentStart(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_'
}

func isIdentContinue(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9')
}

func (g *Generator) collectImports(
	fields []model.OptField,
	localImports map[string]model.ImportInfo,
	needsTypeImports map[string]bool,
) []model.GeneratedImport {
	imports := make(map[string]model.GeneratedImport)

	add := func(path, alias string) {
		if path == "" {
			return
		}
		if existing, ok := imports[path]; ok {
			// Prefer explicit alias when present.
			if existing.Alias == "" && alias != "" {
				imports[path] = model.GeneratedImport{Path: path, Alias: alias}
			}
			return
		}
		imports[path] = model.GeneratedImport{Path: path, Alias: alias}
	}

	parseImportSpec := func(spec string) (path, alias string) {
		// Back-compat / parser encoding: "alias:import/path"
		if i := strings.IndexByte(spec, ':'); i > 0 && i+1 < len(spec) {
			return spec[i+1:], spec[:i]
		}
		return spec, ""
	}

	// localImports is already keyed by the identifier name used in code:
	// - For aliased imports (vaultApi "pkg/api"), key = "vaultApi"
	// - For regular imports ("pkg" or _ "pkg"), key = package name (last path segment)
	// We also build a secondary index by package name for default expressions
	// that may use package names directly.
	importByName := make(map[string]model.ImportInfo, len(localImports)*2)
	for key, imp := range localImports {
		importByName[key] = imp
		// Also index by package name (last path segment) if different from key
		var pathName string
		if idx := strings.LastIndex(imp.Path, "/"); idx >= 0 {
			pathName = imp.Path[idx+1:]
		} else {
			pathName = imp.Path
		}
		if pathName != key {
			if _, exists := importByName[pathName]; !exists {
				importByName[pathName] = imp
			}
		}
	}

	for _, field := range fields {
		// Only include imports for the field type if the generated code references the type
		// (e.g., WithXxx parameter types) or if defaultOptions will reference a non-empty default.
		if needsTypeImports[field.FieldName] || field.Default != "" {
			fieldImports := parser.ExtractImportsFromType(field.TypeExpr, localImports)
			for _, imp := range fieldImports {
				path, alias := parseImportSpec(imp)
				add(path, alias)
			}
		}

		// Extract package prefixes from default expression and add their imports
		if field.Default != "" {
			for _, prefix := range extractPackagePrefixes(field.Default) {
				if imp, ok := importByName[prefix]; ok {
					add(imp.Path, imp.Alias)
				}
			}
		}
	}

	result := make([]model.GeneratedImport, 0, len(imports))
	for _, imp := range imports {
		result = append(result, imp)
	}
	slices.SortFunc(result, func(a, b model.GeneratedImport) int {
		if c := cmp.Compare(a.Path, b.Path); c != 0 {
			return c
		}
		return cmp.Compare(a.Alias, b.Alias)
	})

	return result
}
