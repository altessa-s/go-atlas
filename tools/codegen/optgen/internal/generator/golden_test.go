// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package generator

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	optparser "github.com/altessa-s/go-atlas/tools/codegen/optgen/internal/parser"
)

func TestGenerator_Golden_Default(t *testing.T) {
	t.Parallel()
	runGeneratorGolden(t, false, "testdata/basic", "options_gen.golden")
}

func TestGenerator_Golden_OptionError(t *testing.T) {
	t.Parallel()
	runGeneratorGolden(t, true, "testdata/basic", "options_gen_error.golden")
}

func runGeneratorGolden(t *testing.T, optionReturnsError bool, relDir, goldenName string) {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller failed")
	dir := filepath.Join(filepath.Dir(thisFile), relDir)

	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, nil, parser.ParseComments)
	require.NoError(t, err, "ParseDir")
	pkg, ok := pkgs["basic"]
	require.True(t, ok, "expected package %q in %s", "basic", dir)

	result, err := optparser.FindOptFields(pkg, "options", false)
	require.NoError(t, err, "FindOptFields")

	normalizationMethod := optparser.FindNormalizationMethod(pkg, "options")

	g := New()
	got, err := g.Generate(GenerateInput{
		PackageName:         "basic",
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
}
