// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package architecture_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const module = "github.com/altessa-s/go-atlas/"

// violation checks direct production edges; transitive composition is owned
// by factories. Parse every source file, including inactive platform files.
func violation(pkg, dependency string) bool {
	internal := strings.HasPrefix(dependency, module)
	dep := strings.TrimPrefix(dependency, module)
	switch {
	case strings.HasPrefix(pkg, "core/"):
		if internal {
			return !strings.HasPrefix(dep, "core/")
		}
		if !strings.Contains(strings.Split(dependency, "/")[0], ".") {
			return false
		}
		if pkg == "core/io/spoolbudget" && dependency == "golang.org/x/sync/semaphore" {
			return false
		}
		switch pkg {
		case "core/runtime/capabilities", "core/runtime/landlock", "core/runtime/nonewprivs", "core/runtime/rlimits", "core/runtime/seccomp":
			return dependency != "golang.org/x/sys/unix"
		}
		return true
	case pkg == "config":
		return internal && !strings.HasPrefix(dep, "core/") && !strings.HasPrefix(dep, "config/")
	case strings.HasPrefix(pkg, "config/"):
		return internal && strings.HasPrefix(dep, "transport/")
	case strings.HasPrefix(pkg, "domain/"):
		return internal && !strings.HasPrefix(dep, "core/") && !strings.HasPrefix(dep, "domain/") && !strings.HasPrefix(dep, "observability/")
	case pkg == "observability/tracing", pkg == "observability/metrics":
		return internal && (strings.HasPrefix(dep, "transport/") || strings.HasSuffix(dep, "/factory") || strings.HasPrefix(dep, pkg+"/adapters/"))
	}
	return false
}

func TestProductionBoundaries(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..")
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if strings.HasPrefix(entry.Name(), ".") && path != root {
				return filepath.SkipDir
			}
			switch entry.Name() {
			case "vendor", "tests", "devtools", "graphify-out":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range f.Imports {
			dep, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				return err
			}
			require.False(t, violation(filepath.ToSlash(rel), dep), "%s imports %s; see docs/architecture.md", path, dep)
		}
		return nil
	})
	require.NoError(t, err)
}

func TestBoundaryRules(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, pkg, dep string
		rejected       bool
	}{
		{"core driver", "core/types/optional", "go.mongodb.org/mongo-driver/v2/bson", true},
		{"core inward", "core/context", module + "transport/http/client", true},
		{"core stdlib", "core/context", "context", false},
		{"platform exception", "core/runtime/landlock", "golang.org/x/sys/unix", false},
		{"exception stays narrow", "core/context", "golang.org/x/sys/unix", true},
		{"schema transport", "config", module + "transport/http/client", true},
		{"schema service", "config", module + "service/dispatch", true},
		{"loader secrets", "config/loader/secrets", module + "security/secrets", false},
		{"domain transport", "domain/eventbus", module + "transport/broker", true},
		{"tracer adapter", "observability/tracing", module + "observability/tracing/adapters/otlp", true},
		{"tracer port", "observability/tracing", module + "observability/tracing/adapters", false},
		{"factory composition", "transport/proxydial/factory", module + "config", false},
	} {
		t.Run(tc.name, func(t *testing.T) { t.Parallel(); require.Equal(t, tc.rejected, violation(tc.pkg, tc.dep)) })
	}
}
