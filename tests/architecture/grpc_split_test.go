// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package architecture_test

import (
	"cmp"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestGRPCSplitLayout uses the real import edges between gRPC packages to
// check the planned Go module roots and import rewrites on package stubs.
// Other dependencies are covered by TestModuleBoundaries.
func TestGRPCSplitLayout(t *testing.T) {
	t.Parallel()
	plan, err := newModulePlan()
	require.NoError(t, err)
	roots := []string{"transport", "transport/grpc", "transport/grpc/client", "transport/grpc/handlers/scheduler"}
	relocations := plan.modules["transport"].relocations
	seenDestinations := make(map[string]string, len(relocations))
	for source, destination := range relocations {
		require.DirExists(t, filepath.Join("..", "..", filepath.FromSlash(source)))
		require.Equal(t, "transport", plan.moduleOf(destination), source)
		require.False(t, hasPathPrefix(destination, []string{"transport/grpc"}), source)
		require.NotContains(t, seenDestinations, destination, source)
		seenDestinations[destination] = source
	}

	imports := map[string]map[string]bool{}
	grpcTree := []string{"transport/grpc"}
	walkProductionImports(t, func(_ string, pkg string, dependency string) {
		if !hasPathPrefix(pkg, grpcTree) {
			return
		}
		destination := cmp.Or(relocations[pkg], pkg)
		if imports[destination] == nil {
			imports[destination] = map[string]bool{}
		}
		dep, ok := strings.CutPrefix(dependency, module)
		if !ok || !hasPathPrefix(dep, grpcTree) {
			return
		}
		dep = cmp.Or(relocations[dep], dep)
		if imports[dep] == nil {
			imports[dep] = map[string]bool{}
		}
		imports[destination][dep] = true
	})

	layout := t.TempDir()
	for pkg, deps := range imports {
		var stub strings.Builder
		stub.WriteString("package fixture\n")
		for _, dep := range slices.Sorted(maps.Keys(deps)) {
			fmt.Fprintf(&stub, "import _ %q\n", module+dep)
		}
		writeSplitFile(t, layout, pkg+"/package.go", stub.String())
	}

	for _, root := range roots {
		var manifest strings.Builder
		fmt.Fprintf(&manifest, "module %s\n\ngo 1.27.0\n", module+root)
		for _, dep := range plan.modules[root].deps {
			if slices.Contains(roots, dep) {
				fmt.Fprintf(&manifest, "require %s v0.0.0\n", module+dep)
			}
		}
		for _, dep := range roots {
			if root != dep {
				fmt.Fprintf(&manifest, "replace %s => %q\n", module+dep, filepath.Join(layout, filepath.FromSlash(dep)))
			}
		}
		writeSplitFile(t, layout, root+"/go.mod", manifest.String())
	}

	for _, root := range roots {
		cmd := exec.CommandContext(t.Context(), "go", "list", "-mod=mod", "-deps", "-f", "{{.ImportPath}} {{with .Module}}{{.Path}}{{end}}", "./...")
		cmd.Dir = filepath.Join(layout, filepath.FromSlash(root))
		cmd.Env = append(cmd.Environ(), "GOFLAGS=", "GOWORK=off", "GOTOOLCHAIN=local", "GOPROXY=off", "GOSUMDB=off")
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "%s\n%s", root, out)
		switch root {
		case "transport":
			for _, destination := range relocations {
				require.Contains(t, string(out), module+destination+" "+module+root+"\n")
			}
		case "transport/grpc/client":
			require.Contains(t, string(out), module+"transport/internal/grpcstatus")
			require.NotContains(t, string(out), module+"transport/grpc/interceptors/errstatus")
			require.NotContains(t, string(out), module+"transport/grpc/interceptors/health")
		}
	}
}

func writeSplitFile(t testing.TB, root, name, content string) {
	t.Helper()
	target := filepath.Join(root, filepath.FromSlash(name))
	require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o755))
	require.NoError(t, os.WriteFile(target, []byte(content), 0o600))
}
