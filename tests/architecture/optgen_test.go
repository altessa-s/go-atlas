// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package architecture_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type optgenTarget struct{ name, option string }
type optgenViolation struct{ field, reason string }

func TestOptgenFieldTags(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..")
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != root && (strings.HasPrefix(entry.Name(), ".") || entry.Name() == "vendor" || entry.Name() == "graphify-out") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !strings.Contains(string(data), "//go:generate go tool optgen") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, data, parser.ParseComments)
		if err != nil {
			return err
		}
		targets := optgenTargets(file)
		if len(targets) == 0 {
			return nil
		}
		files, err := optgenPackageFiles(filepath.Dir(path))
		if err != nil {
			return err
		}
		for _, target := range targets {
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			t.Run(filepath.ToSlash(rel)+"/"+target.name, func(t *testing.T) {
				for _, v := range checkOptgenTags(files, target) {
					t.Errorf("%s.%s: %s", target.name, v.field, v.reason)
				}
			})
		}
		return nil
	})
	require.NoError(t, err)
}

func optgenTargets(file *ast.File) []optgenTarget {
	var targets []optgenTarget
	for _, group := range file.Comments {
		for _, comment := range group.List {
			args := strings.Fields(comment.Text)
			if len(args) < 5 || strings.Join(args[:4], " ") != "//go:generate go tool optgen" || args[4] != "generate" {
				continue
			}
			target := optgenTarget{name: "options", option: "Option"}
			for i := 5; i < len(args); i++ {
				key, value, found := strings.Cut(args[i], "=")
				if !found && (key == "--type" || key == "--option-type") && i+1 < len(args) {
					i++
					value = args[i]
				}
				value = strings.Trim(value, "\"")
				switch key {
				case "--type":
					target.name = value
				case "--option-type":
					target.option = value
				}
			}
			targets = append(targets, target)
		}
	}
	return targets
}

func optgenPackageFiles(dir string) ([]*ast.File, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []*ast.File
	fset := token.NewFileSet()
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(dir, entry.Name()), nil, parser.ParseComments)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	return files, nil
}

// Inspect syntax only: no dependency loading or platform-specific type checking.
// Assignment LHSs include compound assignments, map indices and append results.
func optgenWrites(fn *ast.FuncDecl) map[string]bool {
	writes := map[string]bool{}
	if fn.Body == nil {
		return writes
	}
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		assign, ok := node.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for _, lhs := range assign.Lhs {
			recordOptgenWrite(lhs, writes)
		}
		return true
	})
	return writes
}

// Nested selector writes mutate the containing field too (for example,
// c.TransactionOptions.ReadConcern); indices are followed only on the LHS.
func recordOptgenWrite(expr ast.Expr, writes map[string]bool) {
	switch expr := expr.(type) {
	case *ast.SelectorExpr:
		writes[expr.Sel.Name] = true
		recordOptgenWrite(expr.X, writes)
	case *ast.IndexExpr:
		recordOptgenWrite(expr.X, writes)
	case *ast.ParenExpr:
		recordOptgenWrite(expr.X, writes)
	case *ast.StarExpr:
		recordOptgenWrite(expr.X, writes)
	}
}

func returnsOptgenOption(fn *ast.FuncDecl, option string) bool {
	if fn.Type.Results == nil {
		return false
	}
	for _, result := range fn.Type.Results.List {
		expr := result.Type
		switch typ := expr.(type) {
		case *ast.IndexExpr:
			expr = typ.X
		case *ast.IndexListExpr:
			expr = typ.X
		}
		if ident, ok := expr.(*ast.Ident); ok && ident.Name == option {
			return true
		}
	}
	return false
}

func checkOptgenTags(files []*ast.File, target optgenTarget) []optgenViolation {
	manualWrites, optionWrites := map[string]bool{}, map[string]bool{}
	generatedWrites := map[string]bool{}
	var fields []*ast.Field
	for _, file := range files {
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok {
				for field := range optgenWrites(fn) {
					if ast.IsGenerated(file) {
						if fn.Name.IsExported() && returnsOptgenOption(fn, target.option) {
							generatedWrites[field] = true
						}
						continue
					}
					manualWrites[field] = true
					if fn.Name.IsExported() && returnsOptgenOption(fn, target.option) {
						optionWrites[field] = true
					}
				}
			}
			gen, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range gen.Specs {
				typ, ok := spec.(*ast.TypeSpec)
				if !ok || typ.Name.Name != target.name {
					continue
				}
				if st, ok := typ.Type.(*ast.StructType); ok {
					fields = st.Fields.List
				}
			}
		}
	}
	var violations []optgenViolation
	if fields == nil {
		return []optgenViolation{{reason: "optgen target struct not found"}}
	}
	for _, field := range fields {
		raw := ""
		var err error
		if field.Tag != nil {
			raw, err = strconv.Unquote(field.Tag.Value)
		}
		if err != nil {
			violations = append(violations, optgenViolation{reason: "invalid struct tag"})
			continue
		}
		tag := reflect.StructTag(raw)
		directives := strings.Split(tag.Get("optgen"), ",")
		manual, skipped := slices.Contains(directives, "manual"), tag.Get("opt") == "-"
		for _, name := range field.Names {
			var reason string
			switch {
			case slices.Contains(directives, "-"):
				reason = `optgen:"-" is invalid; use opt:"-" or optgen:"manual"`
			case !manual && !skipped && !generatedWrites[name.Name]:
				reason = "field without a generated option needs optgen:\"manual\" or opt:\"-\""
			case manual && skipped:
				reason = "manual and skipped tags are mutually exclusive"
			case manual && !manualWrites[name.Name]:
				reason = "manual field has no non-generated assignment in the package"
			case skipped && optionWrites[name.Name]:
				reason = "exported option assigns skipped field; use optgen:\"manual\""
			case (manual || skipped) && field.Doc == nil:
				reason = "non-generated field needs a comment above it explaining its writer or source"
			}
			if reason != "" {
				violations = append(violations, optgenViolation{name.Name, reason})
			}
		}
	}
	return violations
}

func TestCheckOptgenTags(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, tag, body, generated string
		want                       int
	}{
		{name: "manual assignment", tag: `optgen:"manual"`, body: `func WithValue() Option { return func(o *options) { o.value = 1 } }`},
		{name: "manual helper compound assignment", tag: `optgen:"manual"`, body: `func update(o *options) { o.value += 1 }`},
		{name: "manual map entry", tag: `optgen:"manual"`, body: `func WithValue() Option { return func(o *options) { o.value["key"] = 1 } }`},
		{name: "manual append", tag: `optgen:"manual"`, body: `func WithValue() Option { return func(o *options) { o.value = append(o.value, 1) } }`},
		{name: "nested field", tag: `optgen:"manual"`, body: `func update(o *options) { o.value.member = 1 }`},
		{name: "missing tag", want: 1},
		{name: "generated option", generated: `func WithValue() Option { return func(o *options) { o.value = 1 } }`},
		{name: "manual missing writer", tag: `optgen:"manual"`, want: 1},
		{name: "read is not write", tag: `optgen:"manual"`, body: `func read(o *options) { _ = o.value }`, want: 1},
		{name: "index expression is not write", tag: `optgen:"manual"`, body: `func update(o *options) { other[o.value] = 1 }`, want: 1},
		{name: "generated writer is not manual", tag: `optgen:"manual"`, generated: `func WithValue() Option { return func(o *options) { o.value = 1 } }`, want: 1},
		{name: "skipped internal state", tag: `opt:"-"`, body: `func initialize(o *options) { o.value = 1 }`},
		{name: "skipped generated target", tag: `opt:"-"`, generated: `func WithValue() Option { return func(o *options) { o.value = 1 } }`},
		{name: "skipped option assignment", tag: `opt:"-"`, body: `func WithValue() Option { return func(o *options) { o.value = 1 } }`, want: 1},
		{name: "skipped option map assignment", tag: `opt:"-"`, body: `func WithValue() Option { return func(o *options) { o.value["key"] = 1 } }`, want: 1},
		{name: "other option type", tag: `opt:"-"`, body: `func WithOther() OtherOption { return func(o *otherOptions) { o.value = 1 } }`},
		{name: "invalid optgen dash", tag: `optgen:"-"`, want: 1},
		{name: "conflicting tags", tag: `opt:"-" optgen:"manual"`, want: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			source := "package fixture\ntype options struct {\n// Set by the fixture writer.\nvalue int `" + tt.tag + "`\n}\n" + tt.body
			file, err := parser.ParseFile(token.NewFileSet(), "options.go", source, parser.ParseComments)
			require.NoError(t, err)
			generated, err := parser.ParseFile(token.NewFileSet(), "options_gen.go",
				"// Code generated by optgen. DO NOT EDIT.\npackage fixture\n"+tt.generated, parser.ParseComments)
			require.NoError(t, err)
			violations := checkOptgenTags([]*ast.File{file, generated}, optgenTarget{"options", "Option"})
			require.Len(t, violations, tt.want, "%v", violations)
		})
	}
}

func TestOptgenTargets(t *testing.T) {
	t.Parallel()
	// Split the directive prefix so go generate does not execute fixture text.
	directive := "//go:" + "generate go tool "
	source := "package fixture\n" +
		directive + "optgen generate\n" +
		directive + "optgen generate --type=retryOptions --option-type=RetryOption\n" +
		directive + "optgen generate --type cursorOptions --option-type CursorOption\n" +
		directive + "other generate\n"
	file, err := parser.ParseFile(token.NewFileSet(), "options.go", source, parser.ParseComments)
	require.NoError(t, err)
	require.Equal(t, []optgenTarget{{"options", "Option"}, {"retryOptions", "RetryOption"}, {"cursorOptions", "CursorOption"}}, optgenTargets(file))
}
