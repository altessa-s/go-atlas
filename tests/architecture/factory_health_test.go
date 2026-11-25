// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package architecture_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Factories consume registration ports. Only the coordinator's own factory
// constructs and returns the concrete coordinator.
func TestFactoryHealthDependencies(t *testing.T) {
	t.Parallel()
	walkProductionImports(t, func(file, pkg, dependency string) {
		if !strings.HasPrefix(pkg, "factory/") || pkg == "factory/observability/health" || dependency != module+"observability/health" {
			return
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, file, nil, 0)
		require.NoError(t, err)
		name := "health"
		for _, imp := range f.Imports {
			if imp.Path.Value == strconv.Quote(dependency) && imp.Name != nil {
				name = imp.Name.Name
			}
		}
		for sel := range ast.Preorder(f) {
			ref, ok := sel.(*ast.SelectorExpr)
			if !ok || ref.Sel.Name != "Coordinator" {
				continue
			}
			if id, ok := ref.X.(*ast.Ident); ok && id.Name == name {
				t.Errorf("%s: use the health registration port required by the factory", fset.Position(ref.Pos()))
			}
		}
	})
}
