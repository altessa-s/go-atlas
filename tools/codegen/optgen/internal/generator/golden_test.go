// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package generator

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	optparser "github.com/altessa-s/go-atlas/tools/codegen/optgen/internal/parser"
)

func TestGenerator_Golden_Default(t *testing.T) {
	t.Parallel()
	runGeneratorGolden(t, false, "testdata/basic", "basic", "options_gen.golden")
}

func TestGenerator_Golden_OptionError(t *testing.T) {
	t.Parallel()
	runGeneratorGolden(t, true, "testdata/basic", "basic", "options_gen_error.golden")
}

// The typed fixture covers field types that must be rendered verbatim (fixed
// arrays, func signatures, anonymous structs with tags, interface literals)
// and is type-checked together with the generated options.
func TestGenerator_Golden_Typed(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name               string
		optionReturnsError bool
		golden             string
	}{
		{name: "default", golden: "options_gen.golden"},
		{name: "option error", optionReturnsError: true, golden: "options_gen_error.golden"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := runGeneratorGolden(t, tc.optionReturnsError, "testdata/typed", "typed", tc.golden)
			typeCheckGenerated(t, "testdata/typed", got)
		})
	}
}

// typeCheckGenerated type-checks the fixture sources together with the
// generated file, so the generated code must compile against them.
func typeCheckGenerated(t *testing.T, relDir string, generated []byte) {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller failed")
	dir := filepath.Join(filepath.Dir(thisFile), relDir)

	fset := token.NewFileSet()
	input, err := parser.ParseFile(fset, filepath.Join(dir, "input.go"), nil, 0)
	require.NoError(t, err, "parse input")
	output, err := parser.ParseFile(fset, "options_gen.go", generated, 0)
	require.NoError(t, err, "parse generated output")

	conf := types.Config{Importer: importer.ForCompiler(fset, "source", nil)}
	_, err = conf.Check(input.Name.Name, fset, []*ast.File{input, output}, nil)
	require.NoError(t, err, "generated options must type-check")
}

func runGeneratorGolden(t *testing.T, optionReturnsError bool, relDir, pkgName, goldenName string) []byte {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller failed")
	dir := filepath.Join(filepath.Dir(thisFile), relDir)

	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, nil, parser.ParseComments)
	require.NoError(t, err, "ParseDir")
	pkg, ok := pkgs[pkgName]
	require.True(t, ok, "expected package %q in %s", pkgName, dir)

	result, err := optparser.FindOptFields(pkg, "options", false)
	require.NoError(t, err, "FindOptFields")

	normalizationMethod := optparser.FindNormalizationMethod(pkg, "options")

	g := New()
	got, err := g.Generate(GenerateInput{
		PackageName:         pkgName,
		TypeName:            "options",
		OptionType:          "Option",
		GenerateOptionType:  true,
		GenerateDefaultFunc: true,
		GenerateNewFunc:     true,
		OptionReturnsError:  optionReturnsError,
		Fields:              result.Fields,
		Imports:             result.Imports,
		DefaultFuncName:     "defaultOptions",
		NewFuncName:         "newOptions",
		NormalizationMethod: normalizationMethod,
		GenericInfo:         result.GenericInfo,
	})
	require.NoError(t, err, "Generate")

	goldenPath := filepath.Join(dir, goldenName)
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		require.NoError(t, os.WriteFile(goldenPath, got, 0o644), "write golden")
	}

	want, err := os.ReadFile(goldenPath)
	require.NoError(t, err, "read golden %s (set UPDATE_GOLDEN=1 to create)", goldenPath)

	require.Equal(t, string(want), string(got), "golden mismatch: %s (set UPDATE_GOLDEN=1 to update)", goldenPath)

	return got
}
