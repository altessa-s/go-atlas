// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package architecture_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"
	"github.com/altessa-s/go-atlas/observability/health"

	chconfig "github.com/altessa-s/go-atlas/config/clickhouse"
	meilisearchconfig "github.com/altessa-s/go-atlas/config/meilisearch"
	mongoconfig "github.com/altessa-s/go-atlas/config/mongo"
	natsconfig "github.com/altessa-s/go-atlas/config/nats"
	redisconfig "github.com/altessa-s/go-atlas/config/redis"
	chfactory "github.com/altessa-s/go-atlas/factory/infrastructure/clickhouse"
	meilisearchfactory "github.com/altessa-s/go-atlas/factory/infrastructure/meilisearch"
	mongofactory "github.com/altessa-s/go-atlas/factory/infrastructure/mongo"
	natsfactory "github.com/altessa-s/go-atlas/factory/infrastructure/nats"
	redisfactory "github.com/altessa-s/go-atlas/factory/infrastructure/redis"
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

// Factories must tie health registration to the resource they return.
func TestFactoriesUseScopedHealthRegistration(t *testing.T) {
	t.Parallel()
	walkProductionImports(t, func(file, pkg, dependency string) {
		if !strings.HasPrefix(pkg, "factory/") || dependency != module+"observability/health" {
			return
		}
		// The PostgreSQL connector needs a close hook before the builder can
		// bind its health registration to the returned sql.DB.
		if pkg == "factory/store/postgres" && filepath.Base(file) == "builder.go" {
			return
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, file, nil, 0)
		require.NoError(t, err)
		for node := range ast.Preorder(f) {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				continue
			}
			if method, ok := call.Fun.(*ast.SelectorExpr); ok && method.Sel.Name == "RegisterService" {
				t.Errorf("%s: use a scoped health registration and release it on close", fset.Position(method.Pos()))
			}
		}
	})
}

func TestFactoryCloseRemovesHealthRegistration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		build func(*testing.T, *health.Coordinator) func()
	}{
		{"mongo", func(t *testing.T, c *health.Coordinator) func() {
			t.Helper()
			cfg := mongoconfig.Default()
			cfg.Database = "app"
			m, err := mongofactory.New(&cfg).UseHealthCoordinator(c).Build(t.Context())
			require.NoError(t, err)
			return m.Close
		}},
		{"nats", func(t *testing.T, c *health.Coordinator) func() {
			t.Helper()
			ns := natstest.StartNATSServer(t)
			cfg := natsconfig.Default()
			cfg.Hosts = []string{ns.ClientURL()}
			conn, err := natsfactory.New(&cfg).UseHealthCoordinator(c).Build()
			require.NoError(t, err)
			return conn.Close
		}},
		{"redis", func(t *testing.T, c *health.Coordinator) func() {
			t.Helper()
			cfg := redisconfig.Default()
			client, err := redisfactory.New(&cfg).UseHealthCoordinator(c).SkipStartupPing().Build(t.Context())
			require.NoError(t, err)
			return func() { require.NoError(t, client.Close()) }
		}},
		{"clickhouse", func(t *testing.T, c *health.Coordinator) func() {
			t.Helper()
			cfg := chconfig.Default()
			conn, err := chfactory.New(&cfg).UseHealthCoordinator(c).Build(t.Context())
			require.NoError(t, err)
			return func() { require.NoError(t, conn.Close()) }
		}},
		{"meilisearch", func(t *testing.T, c *health.Coordinator) func() {
			t.Helper()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"status":"available"}`)
			}))
			t.Cleanup(srv.Close)
			cfg := meilisearchconfig.Default()
			cfg.Host = srv.URL
			client, err := meilisearchfactory.New(&cfg).UseHealthCoordinator(c).Build(t.Context())
			require.NoError(t, err)
			return func() { require.NoError(t, client.Close()) }
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := health.New()
			defer c.Close()

			closeFirst := tc.build(t, c)
			t.Cleanup(closeFirst)
			closeReplacement := tc.build(t, c)
			t.Cleanup(closeReplacement)
			require.Contains(t, slices.Collect(c.ListServices()), tc.name)
			closeFirst()
			require.Contains(t, slices.Collect(c.ListServices()), tc.name)
			closeReplacement()
			require.Eventually(t, func() bool {
				return len(slices.Collect(c.ListServices())) == 0
			}, time.Second, time.Millisecond)
		})
	}
}
