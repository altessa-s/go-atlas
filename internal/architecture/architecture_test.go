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

// isSchema reports whether pkg holds configuration schemas: the root config
// package and its subpackages other than the loader, internals and templates.
func isSchema(pkg string) bool {
	if pkg == "config" {
		return true
	}
	rest, ok := strings.CutPrefix(pkg, "config/")
	if !ok {
		return false
	}
	for _, p := range []string{"loader", "internal", "templates"} {
		if rest == p || strings.HasPrefix(rest, p+"/") {
			return false
		}
	}
	return true
}

// isComposition reports whether pkg assembles components from configuration:
// a factory package, or an explicitly designated helper shared by factories.
// Packages nested below a factory are not composition.
func isComposition(pkg string) bool {
	return strings.HasSuffix(pkg, "/factory") || pkg == "transport/internal/factoryconv"
}

// schemaExternal lists the third-party packages schema packages may import:
// validation libraries only.
var schemaExternal = []string{
	"github.com/go-ozzo/ozzo-validation/v4",
	"github.com/altessa-s/ozzo-rules",
}

// allowedSchemaExternal reports whether a schema package may import the
// third-party package dependency.
func allowedSchemaExternal(dependency string) bool {
	for _, p := range schemaExternal {
		if dependency == p || strings.HasPrefix(dependency, p+"/") {
			return true
		}
	}
	return false
}

// edge is a direct production import from pkg to dependency.
type edge struct {
	pkg        string // importing package, relative to the module
	dependency string // imported path as written
	dep        string // imported path relative to the module when internal
	internal   bool   // the dependency belongs to this module
	thirdParty bool   // the dependency is outside the module and the standard library
}

func newEdge(pkg, dependency string) edge {
	dep, internal := strings.CutPrefix(dependency, module)
	// Standard library paths have no dot in their first element.
	thirdParty := !internal && strings.Contains(strings.Split(dependency, "/")[0], ".")
	return edge{pkg: pkg, dependency: dependency, dep: dep, internal: internal, thirdParty: thirdParty}
}

// rule is a named boundary; rejects reports whether the edge crosses it.
type rule struct {
	name    string
	rejects func(e edge) bool
}

// rules are the production boundaries of docs/architecture.md. An edge
// violates the first rule that rejects it.
var rules = []rule{
	{"core depends only on core and approved platform libraries", coreIsolated},
	{"schemas depend only on core, schemas, config internals and validation libraries", schemasDescribeSettings},
	{"only composition reads schemas", runtimeReadsNoSchemas},
	{"only composition depends on factories", runtimeUsesNoFactories},
	{"config internals never reach transport", configStaysOffTransport},
	{"domain depends only on core, domain and observability", domainIsolated},
	{"base observability stays backend-neutral", observabilityNeutral},
}

// violation returns the name of the rule the edge from pkg to dependency
// violates, or "" when it violates none. It checks direct production edges;
// transitive composition is owned by factories.
func violation(pkg, dependency string) string {
	e := newEdge(pkg, dependency)
	for _, r := range rules {
		if r.rejects(e) {
			return r.name
		}
	}
	return ""
}

func coreIsolated(e edge) bool {
	if !strings.HasPrefix(e.pkg, "core/") {
		return false
	}
	switch {
	case e.internal:
		return !strings.HasPrefix(e.dep, "core/")
	case !e.thirdParty:
		return false
	case e.pkg == "core/io/spoolbudget" && e.dependency == "golang.org/x/sync/semaphore":
		return false
	}
	switch e.pkg {
	case "core/runtime/capabilities", "core/runtime/landlock", "core/runtime/nonewprivs", "core/runtime/rlimits", "core/runtime/seccomp":
		return e.dependency != "golang.org/x/sys/unix"
	}
	return true
}

func schemasDescribeSettings(e edge) bool {
	if !isSchema(e.pkg) {
		return false
	}
	if !e.internal {
		return e.thirdParty && !allowedSchemaExternal(e.dependency)
	}
	return !strings.HasPrefix(e.dep, "core/") && !isSchema(e.dep) && e.dep != "config/internal" && !strings.HasPrefix(e.dep, "config/internal/")
}

// runtimeReadsNoSchemas lets config/* (the loader included) read schemas;
// elsewhere only composition does.
func runtimeReadsNoSchemas(e edge) bool {
	return e.internal && isSchema(e.dep) && !isComposition(e.pkg) && !strings.HasPrefix(e.pkg, "config/") && e.pkg != "config"
}

func runtimeUsesNoFactories(e edge) bool {
	return e.internal && isComposition(e.dep) && !isComposition(e.pkg)
}

func configStaysOffTransport(e edge) bool {
	return e.internal && strings.HasPrefix(e.pkg, "config/") && strings.HasPrefix(e.dep, "transport/")
}

func domainIsolated(e edge) bool {
	return e.internal && strings.HasPrefix(e.pkg, "domain/") &&
		!strings.HasPrefix(e.dep, "core/") && !strings.HasPrefix(e.dep, "domain/") && !strings.HasPrefix(e.dep, "observability/")
}

func observabilityNeutral(e edge) bool {
	if e.pkg != "observability/tracing" && e.pkg != "observability/metrics" {
		return false
	}
	return e.internal &&
		(strings.HasPrefix(e.dep, "transport/") || strings.HasSuffix(e.dep, "/factory") || strings.HasPrefix(e.dep, e.pkg+"/adapters/"))
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
			if v := violation(filepath.ToSlash(rel), dep); v != "" {
				t.Errorf("%s imports %s: %s; see docs/architecture.md", path, dep, v)
			}
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
		{"factory helper reads schema", "transport/internal/factoryconv", module + "config", false},
		{"runtime reads schema", "data/limiters/budget", module + "config", true},
		{"runtime reads schema leaf", "auth/oidc", module + "config/proxy", true},
		{"runtime uses composition", "plugins", module + "plugins/factory", true},
		{"factory uses factory", "auth/oidc/factory", module + "transport/proxydial/factory", false},
		{"schema leaf reaches runtime", "config/proxy", module + "transport/proxydial", true},
		{"schema leaf uses leaf", "config/grpc", module + "config/tls", false},
		{"schema leaf uses core", "config/tls", module + "core/types/redacted", false},
		{"loader is not a schema", "config/loader/secrets", module + "security/secrets", false},
		{"loader reads no schema", "config/loader", module + "config", false},
		{"schema uses validation library", "config/grpc", "github.com/go-ozzo/ozzo-validation/v4/is", false},
		{"schema uses stdlib", "config/tls", "crypto/tls", false},
		{"schema uses driver", "config/mongo", "go.mongodb.org/mongo-driver/v2/mongo", true},
		{"schema uses external client", "config", "github.com/redis/go-redis/v9", true},
		{"nested below factory is runtime", "data/foo/factory/runtime", module + "config/grpc", true},
		{"nested below factory uses factory", "data/foo/factory/runtime", module + "data/bar/factory", true},
	} {
		t.Run(tc.name, func(t *testing.T) { t.Parallel(); require.Equal(t, tc.rejected, violation(tc.pkg, tc.dep) != "") })
	}
}

func TestViolationNamesRule(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ pkg, dep, rule string }{
		{"core/context", module + "transport/http/client", "core depends only on core and approved platform libraries"},
		{"config/mongo", "go.mongodb.org/mongo-driver/v2/mongo", "schemas depend only on core, schemas, config internals and validation libraries"},
		{"auth/oidc", module + "config/proxy", "only composition reads schemas"},
		{"plugins", module + "plugins/factory", "only composition depends on factories"},
		{"domain/eventbus", module + "transport/broker", "domain depends only on core, domain and observability"},
		{"observability/tracing", module + "observability/tracing/adapters/otlp", "base observability stays backend-neutral"},
	} {
		t.Run(tc.rule, func(t *testing.T) { t.Parallel(); require.Equal(t, tc.rule, violation(tc.pkg, tc.dep)) })
	}
}
