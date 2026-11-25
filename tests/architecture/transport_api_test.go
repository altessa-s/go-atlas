// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package architecture_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestExternalServerAPI builds and runs the public contract in an unrelated module.
func TestExternalServerAPI(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	root, err := filepath.Abs("../..")
	require.NoError(t, err)
	writeSplitFile(t, dir, "go.mod", fmt.Sprintf("module example.com/serverapi\n\ngo 1.27.0\n\nrequire %s v0.0.0\nreplace %s => %q\n",
		strings.TrimSuffix(module, "/"), strings.TrimSuffix(module, "/"), root))
	src, err := os.ReadFile("testdata/serverapi/server_test.go")
	require.NoError(t, err)
	writeSplitFile(t, dir, "server_test.go", string(src))
	sums, err := os.ReadFile(filepath.Join(root, "go.sum"))
	require.NoError(t, err)
	writeSplitFile(t, dir, "go.sum", string(sums))
	cmd := exec.CommandContext(t.Context(), "go", "test", "-mod=mod", "-count=1", ".")
	cmd.Dir = dir
	cmd.Env = append(cmd.Environ(), "GOWORK=off", "GOPROXY=off", "GOTOOLCHAIN=local")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", out)
}

// A member's identifier includes the public type whose API exposes it.
type transportSymbol struct {
	pkg, ident string
}

// TestTransportPublicAPI checks declarations with their actual Go types. Function
// bodies and private struct fields can freely use implementation packages.
func TestTransportPublicAPI(t *testing.T) {
	t.Parallel()
	cmd := exec.CommandContext(t.Context(), "go", "list", "-deps", "-export", "-json=Dir,DepOnly,GoFiles,ImportPath,Export", "./transport/...")
	cmd.Dir = "../.."
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	require.NoError(t, err, stderr.String())
	type entry struct {
		Dir, ImportPath, Export string
		GoFiles                 []string
		DepOnly                 bool
	}
	exports := map[string]string{}
	var targets []entry
	for dec := json.NewDecoder(bytes.NewReader(out)); dec.More(); {
		var e entry
		require.NoError(t, dec.Decode(&e))
		exports[e.ImportPath] = e.Export
		if !e.DepOnly && !internalPackage(e.ImportPath) {
			targets = append(targets, e)
		}
	}
	require.NotEmpty(t, targets)
	fset := token.NewFileSet()
	importer := importer.ForCompiler(fset, "gc", func(path string) (io.ReadCloser, error) { return os.Open(exports[path]) })
	scan := func(t testing.TB, path string, files []*ast.File, found map[transportSymbol]string) {
		t.Helper()
		info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}}
		cfg := types.Config{Importer: importer}
		pkg, err := cfg.Check(path, fset, files, info)
		require.NoError(t, err)
		check := func(ident string, obj types.Object, typ types.Type) {
			// TypeString visits named types in parameters, results, constraints and
			// nested type arguments without descending into their implementation.
			types.TypeString(typ, func(p *types.Package) string {
				if internalPackage(p.Path()) {
					found[transportSymbol{path, ident}] = fmt.Sprintf("%s exposes a type from %s in %s", fset.Position(obj.Pos()), p.Path(), typ)
				}
				return p.Name()
			})
		}
		for _, name := range pkg.Scope().Names() {
			obj := pkg.Scope().Lookup(name)
			if !obj.Exported() {
				continue
			}
			tn, ok := obj.(*types.TypeName)
			if !ok {
				check(name, obj, obj.Type())
				continue
			}
			if alias, ok := tn.Type().(*types.Alias); ok {
				check(name, obj, alias.Rhs())
				continue
			}
			named := tn.Type().(*types.Named)
			for param := range named.TypeParams().TypeParams() {
				check(name, obj, param.Constraint())
			}
			switch u := named.Underlying().(type) {
			case *types.Struct:
				for f := range u.Fields() {
					if f.Exported() {
						check(name+"."+f.Name(), f, f.Type())
					}
				}
			case *types.Interface:
				for t := range u.EmbeddedTypes() {
					check(name, obj, t)
				}
			default:
				check(name, obj, u)
			}
			// Include pointer and promoted methods, even from private embedded fields.
			methodType := types.Type(named)
			if !types.IsInterface(named) {
				methodType = types.NewPointer(named)
			}
			for method := range types.NewMethodSet(methodType).Methods() {
				if fn := method.Obj(); fn.Exported() {
					check(name+"."+fn.Name(), fn, fn.Type())
				}
			}
		}
		// The underlying type alone cannot distinguish "type Public internal.T"
		// from an independently declared type with the same representation.
		for _, f := range files {
			for spec := range ast.Preorder(f) {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok || !ts.Name.IsExported() || ts.Assign.IsValid() {
					continue
				}
				switch rhs := info.TypeOf(ts.Type).(type) {
				case *types.Named, *types.Alias:
					if obj := pkg.Scope().Lookup(ts.Name.Name); obj != nil && obj.Pos() == ts.Name.Pos() {
						check(obj.Name(), obj, rhs)
					}
				}
			}
		}
	}
	found := map[transportSymbol]string{}
	for _, e := range targets {
		var files []*ast.File
		for _, name := range e.GoFiles {
			f, err := parser.ParseFile(fset, filepath.Join(e.Dir, name), nil, parser.SkipObjectResolution)
			require.NoError(t, err)
			files = append(files, f)
		}
		scan(t, e.ImportPath, files, found)
	}
	for id, detail := range found {
		t.Errorf("%s.%s: %s", id.pkg, id.ident, detail)
	}

	t.Run("fixtures", func(t *testing.T) {
		t.Parallel()
		const path = module + "transport/fixture"
		const source = `package fixture
import "github.com/altessa-s/go-atlas/internal/transport/base"
type Exposed struct { base.Base }
type Clean struct { private base.Base }
func Public() { _ = base.Base{} }
`
		f, err := parser.ParseFile(fset, "fixture.go", source, parser.SkipObjectResolution)
		require.NoError(t, err)
		fixture := map[transportSymbol]string{}
		scan(t, path, []*ast.File{f}, fixture)
		require.Len(t, fixture, 1)
		require.Contains(t, fixture, transportSymbol{path, "Exposed.Base"})
	})
}

func internalPackage(path string) bool { return strings.Contains("/"+path+"/", "/internal/") }
