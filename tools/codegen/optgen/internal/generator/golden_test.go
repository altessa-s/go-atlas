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
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Join(filepath.Dir(thisFile), relDir)

	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("ParseDir: %v", err)
	}
	pkg, ok := pkgs["basic"]
	if !ok {
		t.Fatalf("expected package %q in %s", "basic", dir)
	}

	result, err := optparser.FindOptFields(pkg, "options", false)
	if err != nil {
		t.Fatalf("FindOptFields: %v", err)
	}

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
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	goldenPath := filepath.Join(dir, goldenName)
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden %s: %v (set UPDATE_GOLDEN=1 to create)", goldenPath, err)
	}

	if string(got) != string(want) {
		t.Fatalf("golden mismatch: %s (set UPDATE_GOLDEN=1 to update)", goldenPath)
	}
}
