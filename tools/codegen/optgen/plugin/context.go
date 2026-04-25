// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package plugin

import (
	"cmp"
	"slices"

	"github.com/altessa-s/go-atlas/tools/codegen/optgen/model"
)

// GenerationContext is passed to every plugin during code generation. It carries
// package-level metadata (package name, type name, generic params) and allows
// plugins to register additional imports via [GenerationContext.AddImport].
type GenerationContext struct {
	// PackageName is the name of the package being generated.
	PackageName string

	// TypeName is the name of the options struct type.
	TypeName string

	// OptionType is the name of the Option function type.
	OptionType string

	// OptionReturnsError indicates whether Option functions return an error:
	//   type Option func(*options) error
	OptionReturnsError bool

	// AllFields contains all fields being generated.
	AllFields []model.OptField

	// Imports contains all imports available in the source file.
	// Map key is the package alias, value is import info.
	Imports map[string]model.ImportInfo

	// GenericInfo contains generic type parameters if the struct is generic.
	GenericInfo model.GenericInfo

	// additionalImports tracks imports added by plugins.
	additionalImports map[string]model.GeneratedImport
}

// NewGenerationContext creates a GenerationContext with an empty additional-imports map.
// Callers should populate it once per generation run.
func NewGenerationContext(
	pkgName, typeName, optionType string,
	optionReturnsError bool,
	fields []model.OptField,
	imports map[string]model.ImportInfo,
	genericInfo model.GenericInfo,
) GenerationContext {
	return GenerationContext{
		PackageName:        pkgName,
		TypeName:           typeName,
		OptionType:         optionType,
		OptionReturnsError: optionReturnsError,
		AllFields:          fields,
		Imports:            imports,
		GenericInfo:        genericInfo,
		additionalImports:  make(map[string]model.GeneratedImport),
	}
}

// IsGeneric returns true if generating for a generic struct.
func (ctx *GenerationContext) IsGeneric() bool {
	return ctx.GenericInfo.IsGeneric()
}

// TypeParamsDecl returns the type parameters declaration string.
// Example: "[T any]" or "" if not generic.
func (ctx *GenerationContext) TypeParamsDecl() string {
	return ctx.GenericInfo.TypeParamsDecl()
}

// TypeParamsNames returns just the type parameter names.
// Example: "[T]" or "" if not generic.
func (ctx *GenerationContext) TypeParamsNames() string {
	return ctx.GenericInfo.TypeParamsNames()
}

// AddImport adds an import to the generation context.
// The import will be included in the generated file.
//
// If alias is provided, it will be used as the import alias.
// Otherwise, the last segment of the path is used.
func (ctx *GenerationContext) AddImport(path string, alias ...string) {
	imp := model.GeneratedImport{Path: path}
	if len(alias) > 0 && alias[0] != "" {
		imp.Alias = alias[0]
	}
	// Deduplicate by path. Prefer explicit alias when provided.
	if existing, ok := ctx.additionalImports[path]; ok {
		if existing.Alias != "" && imp.Alias == "" {
			return
		}
	}
	ctx.additionalImports[path] = imp
}

// GetAdditionalImports returns a deterministically-sorted slice of all imports
// added by plugins via [GenerationContext.AddImport]. The slice is sorted by
// import path, then alias.
func (ctx *GenerationContext) GetAdditionalImports() []model.GeneratedImport {
	imports := make([]model.GeneratedImport, 0, len(ctx.additionalImports))
	for _, imp := range ctx.additionalImports {
		imports = append(imports, imp)
	}
	slices.SortFunc(imports, func(a, b model.GeneratedImport) int {
		if c := cmp.Compare(a.Path, b.Path); c != 0 {
			return c
		}
		return cmp.Compare(a.Alias, b.Alias)
	})
	return imports
}
