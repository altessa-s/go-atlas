// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package architecture_test

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// factoryOwnTrees are the factory subtrees that build no package of their
// own path: the generic base, the client lifecycles of external systems and
// the helpers shared by the reliability and transport factories.
var factoryOwnTrees = []string{"core", "infrastructure", "reliability/internal", "transport/internal"}

// factoryExtModules maps the first segment under factory/ext to the external
// module whose packages the factories there build.
var factoryExtModules = map[string]bool{
	"aip":      true, // go-aip
	"core":     true, // go-atlas-core
	"net":      true, // go-atlas-net
	"horai":    true, // go-horai
	"kryptos":  true, // go-kryptos
	"slogx":    true, // go-slogx
	"tlsutils": true, // go-tlsutils
}

// TestFactoryMirrorsTree pins the layout rule that factory/<path> builds
// <path>: every Go package under factory/ either has a package of the same
// path in this module, belongs to one of factoryOwnTrees, or sits under
// factory/ext/<module> for an external module of factoryExtModules.
func TestFactoryMirrorsTree(t *testing.T) {
	t.Parallel()

	root := filepath.Join("..", "..")
	var orphans []string
	err := filepath.WalkDir(filepath.Join(root, "factory"), func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() || !hasGoSources(p) {
			return err
		}
		rel, err := filepath.Rel(filepath.Join(root, "factory"), p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "." || hasPathPrefix(rel, factoryOwnTrees) {
			return nil
		}
		if module, ok := strings.CutPrefix(rel, "ext/"); ok {
			if !factoryExtModules[strings.Split(module, "/")[0]] {
				orphans = append(orphans, path.Join("factory", rel))
			}
			return nil
		}
		if info, statErr := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); statErr != nil || !info.IsDir() {
			orphans = append(orphans, path.Join("factory", rel))
		}
		return nil
	})
	require.NoError(t, err)
	require.Empty(t, orphans, "factory packages without a package of the same path; move them, or list their tree in factoryOwnTrees or factoryExtModules")
}

// hasGoSources reports whether dir holds a non-test Go file.
func hasGoSources(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if name := e.Name(); strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") {
			return true
		}
	}
	return false
}
