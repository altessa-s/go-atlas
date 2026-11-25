// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package architecture_test

import (
	"bytes"
	"encoding/json"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTransportErrorLogging(t *testing.T) {
	t.Parallel()
	// Use compiled dependency types so a selector, alias, promoted method,
	// or an error returned directly by a call is checked by its Go type.
	cmd := exec.CommandContext(t.Context(), "go", "list", "-deps", "-export",
		"-json=ImportPath,Dir,GoFiles,Export,DepOnly", "./transport/...", "./internal/transport/...")
	cmd.Dir = "../.."
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	require.NoError(t, err, stderr.String())
	type listEntry struct {
		ImportPath, Dir, Export string
		GoFiles                 []string
		DepOnly                 bool
	}
	var targets []listEntry
	exports := map[string]string{}
	dec := json.NewDecoder(bytes.NewReader(out))
	for dec.More() {
		var pkg listEntry
		require.NoError(t, dec.Decode(&pkg))
		exports[pkg.ImportPath] = pkg.Export
		if !pkg.DepOnly {
			targets = append(targets, pkg)
		}
	}
	require.NotEmpty(t, targets)
	fset := token.NewFileSet()
	cfg := types.Config{Importer: importer.ForCompiler(fset, "gc", func(path string) (io.ReadCloser, error) {
		return os.Open(exports[path])
	})}

	check := func(t *testing.T, path string, files []*ast.File) []int {
		t.Helper()
		info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}, Uses: map[*ast.Ident]types.Object{}}
		_, err := cfg.Check(path, fset, files, info)
		require.NoError(t, err)
		var lines []int
		for _, f := range files {
			for call := range ast.Preorder(f) {
				c, ok := call.(*ast.CallExpr)
				if !ok || !transportLogSink(c.Fun, info) {
					continue
				}
				for _, arg := range c.Args {
					if rawLogValue(arg, info, true) {
						pos := fset.Position(arg.Pos())
						lines = append(lines, pos.Line)
						if path != "fixture" {
							t.Errorf("%s: raw error passed to transport log", pos)
						}
					}
				}
			}
		}
		return lines
	}

	t.Run("fixtures", func(t *testing.T) {
		const source = `package fixture
import (
	"context"
	"fmt"
	log "log/slog"
 "strings"
	d "github.com/altessa-s/go-atlas/internal/diagnostics"
	"github.com/altessa-s/go-atlas/internal/transport/base"
)
type payload struct{ Cause error }
type concrete struct{}
func (*concrete) Error() string { return "problem" }
func run(ctx context.Context, l *log.Logger, b *base.Base, p payload, failure func() error) {
	log.Warn("msg", "detail", p.Cause) // want
	l.With("detail", failure()) // want
	log.Group("state", "result", new(concrete)) // want
	log.Info("msg", "result", error.Error(p.Cause)) // want
	log.Any("detail", p.Cause) // want
	l.ErrorContext(ctx, p.Cause.Error()) // want
	log.String("detail", fmt.Sprintf("cause %v", p.Cause)) // want
	b.LogWarn(ctx, "msg", "op", failure()) // want
	l.Info("msg", "detail", fmt.Sprint(p.Cause)) // want
	l.Info("msg", "detail", fmt.Appendf(nil,"%v",p.Cause)) // want
	log.String("reason", strings.TrimSpace(p.Cause.Error())) // want
	log.Any("result", []any{p.Cause}) // want
	l.With("detail", d.LogError(p.Cause))
	b.LogWarn(ctx, "msg", "op", d.LogError(failure()))
	l.Info("msg", "error", "fixed", "classification", d.ErrorClass(p.Cause))
}`
		var want []int
		for i, line := range strings.Split(source, "\n") {
			if strings.Contains(line, "// want") {
				want = append(want, i+1)
			}
		}
		f, err := parser.ParseFile(fset, "fixture.go", source, 0)
		require.NoError(t, err)
		require.Equal(t, want, check(t, "fixture", []*ast.File{f}))
	})
	scanned := map[string]bool{}
	for _, pkg := range targets {
		var files []*ast.File
		for _, name := range pkg.GoFiles {
			path := filepath.Join(pkg.Dir, name)
			scanned[path] = true
			f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
			require.NoError(t, err)
			files = append(files, f)
		}
		check(t, pkg.ImportPath, files)
	}
	// A new build constraint must not silently remove a production file
	// from the rule. Report any file needing an additional build profile.
	root, err := filepath.Abs(cmd.Dir)
	require.NoError(t, err)
	for _, tree := range []string{"transport", "internal/transport"} {
		err := fs.WalkDir(os.DirFS(root), tree, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && d.Name() == "testdata" {
				return fs.SkipDir
			}
			if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") && !scanned[filepath.Join(root, path)] {
				t.Errorf("%s: production file missing from transport log check", path)
			}
			return nil
		})
		require.NoError(t, err)
	}
}

func logFunction(expr ast.Expr, info *types.Info) *types.Func {
	var id *ast.Ident
	switch fn := ast.Unparen(expr).(type) {
	case *ast.Ident:
		id = fn
	case *ast.SelectorExpr:
		id = fn.Sel
	default:
		return nil
	}
	fn, _ := info.Uses[id].(*types.Func)
	return fn
}

func transportLogSink(expr ast.Expr, info *types.Info) bool {
	fn := logFunction(expr, info)
	if fn == nil || fn.Pkg() == nil {
		return false
	}
	switch fn.Pkg().Path() {
	case "log/slog":
		return true
	case module + "internal/transport/base", module + "transport/grpc/interceptors", module + "transport/http/server/middlewares":
		return strings.HasPrefix(fn.Name(), "Log")
	case "github.com/altessa-s/go-slogx":
		return fn.Name() == "Error"
	}
	return false
}

// rendered distinguishes an error used as a log value from one passed to
// a classifier that returns a safe value. Explicit formatting inside an
// attribute expression is checked in either case.
func rawLogValue(expr ast.Expr, info *types.Info, rendered bool) bool {
	expr = ast.Unparen(expr)
	call, isCall := expr.(*ast.CallExpr)
	var fn *types.Func
	if isCall {
		fn = logFunction(call.Fun, info)
		if fn != nil && fn.Pkg() != nil && fn.Pkg().Path() == module+"internal/diagnostics" && fn.Name() == "LogError" {
			return false
		}
	}
	typ := info.TypeOf(expr)
	errorType := types.Universe.Lookup("error").Type().Underlying().(*types.Interface)
	if rendered && typ != nil && typ != types.Typ[types.UntypedNil] && types.Implements(typ, errorType) {
		return true
	}
	switch e := expr.(type) {
	case *ast.BinaryExpr:
		return e.Op == token.ADD && (rawLogValue(e.X, info, rendered) || rawLogValue(e.Y, info, rendered))
	case *ast.CallExpr:
		if sel, ok := e.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Error" {
			switch len(e.Args) {
			case 0:
				return rawLogValue(sel.X, info, true)
			case 1:
				return info.Types[sel.X].IsType() && rawLogValue(e.Args[0], info, true)
			}
		}
		formats := fn != nil && fn.Pkg() != nil && fn.Pkg().Path() == "fmt" &&
			(strings.HasPrefix(fn.Name(), "Sprint") || strings.HasPrefix(fn.Name(), "Append"))
		return slices.ContainsFunc(e.Args, func(arg ast.Expr) bool {
			return rawLogValue(arg, info, formats || (rendered && info.Types[e.Fun].IsType()))
		})
	case *ast.CompositeLit:
		return slices.ContainsFunc(e.Elts, func(elt ast.Expr) bool { return rawLogValue(elt, info, rendered) })
	case *ast.KeyValueExpr:
		return rawLogValue(e.Value, info, rendered)
	}
	return false
}
