// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package architecture_test

import (
	"maps"
	"path"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// targetModule is one module of the planned multi-module layout: a
// directory that will carry its own go.mod. extra lists directories outside
// root that move into the module when it is split off. deps are the modules
// it requires directly; a module may import any module they reach.
type targetModule struct {
	root  string
	extra []string
	deps  []string
	// drivers lets a non-backend module import driver libraries (the
	// in-process test servers).
	drivers bool
}

// baseModules are the API, schema and runtime modules. They never import a
// driver or heavy SDK; a backend reaches them by registration.
var baseModules = []targetModule{
	{root: "core"},
	{root: "internal/testhelpers", deps: []string{"core", "service/scheduler/task"}},
	{root: "internal/testhelpers/servers", deps: []string{"core"}, drivers: true},
	{root: "internal/sqldialect"},
	{root: "internal/purgeloop", deps: []string{"core"}},
	{root: "auth/principal"},
	{root: "service/scheduler/task", deps: []string{"core"}},
	{root: "service/scheduler/schedule", deps: []string{"core"}},
	{root: "cache/lru", deps: []string{"core"}},
	{root: "security/secrets/sealbox", deps: []string{"core"}},

	{root: "config", deps: []string{"core", "net", "service/scheduler/task", "service/scheduler/schedule", "internal/sqldialect"}},
	{root: "observability", deps: []string{"core", "config", "service/scheduler/task"}},
	{root: "observability/metrics/adapters/prometheus", deps: []string{"observability"}},
	{root: "observability/tracing/adapters/otlp", deps: []string{"observability", "net/http", "net/grpc", "transport/grpc/client"}},
	{root: "domain", extra: []string{"proto/gen"}, deps: []string{"core", "observability"}},
	{root: "cache", deps: []string{"core", "observability", "cache/lru"}},
	{root: "mapping", deps: []string{"core", "domain"}},

	{root: "net", deps: []string{"core", "cache/lru"}},
	{root: "net/grpc", deps: []string{"core", "net"}},
	{root: "net/http", deps: []string{"core", "net", "observability", "config"}},
	{
		root:  "transport",
		extra: []string{"internal/transport"},
		deps:  []string{"core", "observability", "config", "net", "net/grpc", "cache", "coord", "reliability", "audit", "auth/principal", "internal/testhelpers"},
	},
	{root: "transport/grpc/client", deps: []string{"core", "observability", "config", "transport", "net", "net/grpc"}},
	{root: "internal/memcleanup", deps: []string{"core", "service/scheduler/task"}},
	{root: "coord", deps: []string{"core", "observability", "cache/lru", "internal/memcleanup", "internal/sqldialect", "internal/purgeloop"}},
	{root: "audit", deps: []string{"core", "observability", "internal/sqldialect", "internal/purgeloop"}},
	{root: "reliability", deps: []string{"core", "observability", "auth/principal", "internal/memcleanup", "internal/sqldialect", "internal/purgeloop"}},
	{root: "auth", deps: []string{"core", "config", "observability", "cache", "audit", "auth/principal", "net/http"}},
	{root: "security", deps: []string{"core", "config", "observability"}},
	{root: "transport/http", deps: []string{"transport", "net", "auth", "security", "domain"}},
	{root: "transport/grpc", deps: []string{"transport", "transport/grpc/client", "net", "net/grpc", "auth", "security", "domain"}},
	// The gRPC adapter of the go-horai scheduler stays off the gRPC module.
	{root: "transport/grpc/handlers/scheduler", deps: []string{"transport/grpc", "service/scheduler"}},
	{root: "messaging/eventbus", deps: []string{"core", "observability"}},
	{root: "messaging/broker", deps: []string{"core", "config", "observability", "reliability"}},
	{
		root:  "service/scheduler",
		extra: []string{"proto/gen/scheduler"},
		deps:  []string{"core", "config", "observability", "domain", "internal/sqldialect"},
	},

	// factory is the composition layer: the driver-neutral factories, which
	// may import every runtime module but no driver backend or SDK leaf.
	{root: "factory", deps: []string{
		"core", "config", "observability", "domain", "cache", "coord", "reliability", "audit", "net", "net/grpc", "net/http", "transport",
		"transport/grpc/client", "auth", "security", "transport/http", "transport/grpc", "messaging/broker", "service/scheduler",
	}},
	{root: "factory/observability/metrics/adapters/prometheus", deps: []string{"observability/metrics/adapters/prometheus", "factory"}},
	{root: "factory/observability/tracing/adapters/otlp", deps: []string{"observability/tracing/adapters/otlp", "factory"}},
}

// sdkModules are the leaves carrying a heavy SDK (OPA, Vault, cloud
// secret managers, S3), each factory of one its own leaf next to it.
var sdkModules = []targetModule{
	{root: "auth/opa", deps: []string{"auth", "net/http"}},
	{root: "factory/auth/opa", deps: []string{"auth/opa", "factory"}},
	{root: "security/vault", deps: []string{"security"}},
	{root: "factory/security/vault", deps: []string{"security/vault", "factory"}},
	{root: "security/secrets/providers/gcp", deps: []string{"security"}},
	{root: "factory/ext/kryptos/gcp", deps: []string{"security/secrets/providers/gcp", "factory"}},
	{root: "security/secrets/providers/lockbox", deps: []string{"security"}},
	{root: "factory/ext/kryptos/lockbox", deps: []string{"security/secrets/providers/lockbox", "factory"}},
	{root: "security/secrets/providers/vault", deps: []string{"security", "security/vault"}},
	{root: "factory/ext/kryptos/vault", deps: []string{"security/secrets/providers/vault", "factory"}},
	{root: "security/tlsutils/providers/s3", deps: []string{"security"}},
	{root: "factory/ext/tlsutils/s3", deps: []string{"security/tlsutils/providers/s3", "factory"}},
	{root: "security/tlsutils/providers/vault", deps: []string{"security", "security/vault"}},
	{root: "factory/ext/tlsutils/vault", deps: []string{"security/tlsutils/providers/vault", "factory"}},
}

// driverBases are the modules shared by the backends of one driver.
var driverBases = []targetModule{
	{root: "store/postgres", deps: []string{"core", "observability", "internal/sqldialect"}},
	{root: "factory/store/postgres", deps: []string{"store/postgres", "factory"}},
	{root: "store/mongo", deps: []string{"core", "observability", "internal/memcleanup"}},
	{root: "factory/store/mongo", deps: []string{"store/mongo", "factory"}},
	{root: "store/meilisearch", deps: []string{"core"}},
	{root: "internal/redis", deps: []string{"core"}},
	// The RedisBloom plumbing shared by the Bloom and Cuckoo redis backends.
	{root: "cache/probfilter/internal/redisfilter", deps: []string{"core", "cache", "internal/redis"}},
	{root: "internal/nats", deps: []string{"core", "observability"}},
	{root: "factory/infrastructure/mongo", deps: []string{"factory", "store/mongo"}},
	{root: "factory/infrastructure/redis", deps: []string{"factory"}},
	{root: "factory/infrastructure/nats", deps: []string{"factory"}},
	{root: "factory/infrastructure/clickhouse", deps: []string{"factory"}},
	{root: "factory/infrastructure/meilisearch", deps: []string{"factory", "store/meilisearch"}},
}

// driverBackends maps each driver base to the backend modules built on it.
// A backend requires its base and the module of its parent directory.
var driverBackends = map[string][]string{
	"store/mongo": {
		"audit/storages/mongo", "reliability/idempotency/storages/mongo", "coord/dlock/providers/mongo", "reliability/outbox/storages/mongo", "reliability/retryqueue/storages/mongo",
		"reliability/saga/storages/mongo", "mapping/behavior/translators/mongo", "service/scheduler/storages/mongo",
	},
	"internal/redis": {
		"auth/denylist/storages/redis", "cache/storages/redis", "cache/generation/storages/redis",
		"reliability/idempotency/storages/redis", "coord/limiters/storages/redis", "store/mongo/storages/redis",
		"reliability/saga/storages/redis", "service/scheduler/storages/redis",
	},
	"cache/probfilter/internal/redisfilter": {"cache/probfilter/bloom/storages/redis", "cache/probfilter/cuckoo/storages/redis"},
	"internal/nats": {
		"reliability/idempotency/storages/nats", "coord/leadelect/providers/nats", "coord/limiters/storages/nats",
		"coord/dlock/providers/nats", "store/mongo/storages/nats", "reliability/saga/engines/jetstream",
		"reliability/saga/storages/nats", "messaging/broker/providers/nats",
	},
	"factory/infrastructure/clickhouse": {"audit/storages/clickhouse", "service/scheduler/storages/clickhouse"},
}

// backendFactories maps each driver backend with a factory to that factory.
// A backend factory is a leaf of its own: it requires the backend and the
// module of its parent directory, the factory it registers on.
var backendFactories = map[string]string{
	"audit/storages/clickhouse":              "factory/audit/storages/clickhouse",
	"audit/storages/mongo":                   "factory/audit/storages/mongo",
	"cache/generation/storages/redis":        "factory/cache/generation/storages/redis",
	"cache/storages/redis":                   "factory/cache/storages/redis",
	"reliability/idempotency/storages/mongo": "factory/reliability/idempotency/storages/mongo",
	"reliability/idempotency/storages/nats":  "factory/reliability/idempotency/storages/nats",
	"reliability/idempotency/storages/redis": "factory/reliability/idempotency/storages/redis",
	"coord/leadelect/providers/nats":         "factory/coord/leadelect/providers/nats",
	"coord/limiters/storages/nats":           "factory/coord/limiters/storages/nats",
	"coord/limiters/storages/redis":          "factory/coord/limiters/storages/redis",
	"coord/dlock/providers/mongo":            "factory/coord/dlock/providers/mongo",
	"coord/dlock/providers/nats":             "factory/coord/dlock/providers/nats",
	"store/mongo/storages/nats":              "factory/store/mongo/storages/nats",
	"store/mongo/storages/redis":             "factory/store/mongo/storages/redis",
	"reliability/outbox/storages/mongo":      "factory/reliability/outbox/storages/mongo",
	"reliability/retryqueue/storages/mongo":  "factory/reliability/retryqueue/storages/mongo",
	"cache/probfilter/bloom/storages/redis":  "factory/cache/probfilter/bloom/storages/redis",
	"cache/probfilter/cuckoo/storages/redis": "factory/cache/probfilter/cuckoo/storages/redis",
	"reliability/saga/storages/mongo":        "factory/reliability/saga/storages/mongo",
	"reliability/saga/storages/nats":         "factory/reliability/saga/storages/nats",
	"reliability/saga/storages/redis":        "factory/reliability/saga/storages/redis",
	"service/scheduler/storages/clickhouse":  "factory/ext/horai/clickhouse",
	"service/scheduler/storages/mongo":       "factory/ext/horai/mongo",
	"service/scheduler/storages/redis":       "factory/ext/horai/redis",
	"messaging/broker/providers/nats":        "factory/messaging/broker/providers/nats",
}

// moduleViolations are the module edges the code still has against the
// target layout. The list only shrinks: a new violation fails the test, and
// so does an entry that no longer occurs.
var moduleViolations = map[string]bool{
	// The gRPC client shares the server's interceptors (MODULARIZATION_PLAN §13.4).
	"transport/grpc/client -> transport/grpc": true,
}

// modulePlan is the resolved target layout.
type modulePlan struct {
	modules map[string]targetModule
	reach   map[string]map[string]bool
}

func newModulePlan() (*modulePlan, error) {
	p := &modulePlan{modules: map[string]targetModule{}}
	for _, set := range [][]targetModule{baseModules, sdkModules, driverBases} {
		for _, m := range set {
			p.modules[m.root] = m
		}
	}
	for _, base := range slices.Sorted(maps.Keys(driverBackends)) {
		for _, root := range driverBackends[base] {
			parent := p.moduleOf(path.Dir(root))
			p.modules[root] = targetModule{root: root, deps: []string{base, parent}}
		}
	}
	for _, backend := range slices.Sorted(maps.Keys(backendFactories)) {
		root := backendFactories[backend]
		p.modules[root] = targetModule{root: root, deps: []string{backend, p.moduleOf(path.Dir(root))}, drivers: true}
	}
	p.reach = map[string]map[string]bool{}
	for root := range p.modules {
		if err := p.resolve(root, nil); err != nil {
			return nil, err
		}
	}
	return p, nil
}

// resolve computes the modules root reaches, failing on a cycle or an
// unknown dependency.
func (p *modulePlan) resolve(root string, stack []string) error {
	if _, done := p.reach[root]; done {
		return nil
	}
	if slices.Contains(stack, root) {
		return &cycleError{path: append(stack, root)}
	}
	m, ok := p.modules[root]
	if !ok {
		return &unknownModuleError{module: root, from: stack}
	}
	reach := map[string]bool{}
	for _, dep := range m.deps {
		if err := p.resolve(dep, append(stack, root)); err != nil {
			return err
		}
		reach[dep] = true
		maps.Copy(reach, p.reach[dep])
	}
	p.reach[root] = reach
	return nil
}

// moduleOf returns the module holding pkg: the longest module root or extra
// directory that is pkg or one of its parents, or "" when none is.
func (p *modulePlan) moduleOf(pkg string) string {
	best, bestLen := "", -1
	for root, m := range p.modules {
		for _, dir := range append([]string{root}, m.extra...) {
			if len(dir) > bestLen && hasPathPrefix(pkg, []string{dir}) {
				best, bestLen = root, len(dir)
			}
		}
	}
	return best
}

// allows reports whether module from may import module to.
func (p *modulePlan) allows(from, to string) bool {
	return from == to || p.reach[from][to]
}

type cycleError struct{ path []string }

func (e *cycleError) Error() string { return "module cycle: " + strings.Join(e.path, " -> ") }

type unknownModuleError struct {
	module string
	from   []string
}

func (e *unknownModuleError) Error() string {
	return "unknown module " + e.module + " required by " + strings.Join(e.from, " -> ")
}

// isDriverModule reports whether root is a driver base, a driver backend, an
// SDK leaf or marked drivers: the only modules that may import driver and SDK
// libraries.
func (p *modulePlan) isDriverModule(root string) bool {
	if p.modules[root].drivers {
		return true
	}
	if slices.ContainsFunc(driverBases, func(m targetModule) bool { return m.root == root }) ||
		slices.ContainsFunc(sdkModules, func(m targetModule) bool { return m.root == root }) {
		return true
	}
	for _, backends := range driverBackends {
		if slices.Contains(backends, root) {
			return true
		}
	}
	return false
}

// driverLibraries are the third-party driver and heavy SDK libraries only
// driver modules import.
var driverLibraries = []string{
	"github.com/jackc/pgx",
	"go.mongodb.org/mongo-driver",
	"github.com/redis/go-redis",
	"github.com/nats-io/nats.go",
	"github.com/ClickHouse/clickhouse-go",
	"github.com/meilisearch/meilisearch-go",
	"github.com/open-policy-agent/opa",
	"github.com/hashicorp/vault/api",
	"github.com/aws/aws-sdk-go-v2",
	"cloud.google.com/go/secretmanager",
	"github.com/yandex-cloud/go-genproto",
}

// moduleEdge is one production import seen as a module dependency.
type moduleEdge struct {
	from, to string
}

// checkModuleEdge returns why importing dependency from pkg breaks the
// target layout, or "" when it does not.
func (p *modulePlan) checkModuleEdge(pkg, dependency string) (string, moduleEdge) {
	from := p.moduleOf(pkg)
	if from == "" {
		return "package belongs to no target module", moduleEdge{}
	}
	if dep, internal := internalPath(dependency); internal {
		to := p.moduleOf(dep)
		if to == "" {
			return "dependency belongs to no target module", moduleEdge{}
		}
		if !p.allows(from, to) {
			return "module " + from + " does not require " + to, moduleEdge{from: from, to: to}
		}
		return "", moduleEdge{}
	}
	if !p.isDriverModule(from) && hasPathPrefix(dependency, driverLibraries) {
		return "module " + from + " imports a driver library", moduleEdge{from: from, to: dependency}
	}
	return "", moduleEdge{}
}

func TestModulePlanIsDAG(t *testing.T) {
	t.Parallel()
	_, err := newModulePlan()
	require.NoError(t, err)
}

func TestModulePlanDetectsCycle(t *testing.T) {
	t.Parallel()
	p := &modulePlan{
		modules: map[string]targetModule{"a": {root: "a", deps: []string{"b"}}, "b": {root: "b", deps: []string{"a"}}},
		reach:   map[string]map[string]bool{},
	}
	err := p.resolve("a", nil)
	_, ok := err.(*cycleError) //nolint:errorlint // resolve returns the error unwrapped
	require.True(t, ok, "got %v", err)
}

func TestModuleOf(t *testing.T) {
	t.Parallel()
	p, err := newModulePlan()
	require.NoError(t, err)
	tests := map[string]string{
		"core/errors":                                    "core",
		"internal/testhelpers/servers/natstest":          "internal/testhelpers/servers",
		"reliability/idempotency":                        "reliability",
		"factory/reliability/idempotency/storages/redis": "factory/reliability/idempotency/storages/redis",
		"factory/reliability/idempotency":                "factory",
		"factory/store/mongo/kms":                        "factory/store/mongo",
		"factory/store/mongo/storages/nats":              "factory/store/mongo/storages/nats",
		"factory/transport/internal/factoryconv":         "factory",
		"factory/ext/net/httpclient/observe":             "factory",
		"internal/transport/server":                      "transport",
		"internal/redis/redisbase":                       "internal/redis",
		"cache/probfilter/internal/redisfilter":          "cache/probfilter/internal/redisfilter",
		"cache/probfilter/internal/facade":               "cache",
		"store/mongo/storages/nats":                      "store/mongo/storages/nats",
		"store/mongo/kms":                                "store/mongo",
		"proto/gen/scheduler/v1":                         "service/scheduler",
		"proto/gen/fieldbehaviortest":                    "domain",
		"transport/grpc/handlers/scheduler":              "transport/grpc/handlers/scheduler",
		"factory/ext/kryptos/vault":                      "factory/ext/kryptos/vault",
		"security/secrets/providers/memory":              "security",
		"observability/metrics/adapters/prometheus/x":    "observability/metrics/adapters/prometheus",
		"devtools": "",
	}
	for pkg, want := range tests {
		require.Equal(t, want, p.moduleOf(pkg), pkg)
	}
}

func TestCheckModuleEdge(t *testing.T) {
	t.Parallel()
	p, err := newModulePlan()
	require.NoError(t, err)
	tests := []struct {
		name, pkg, dep string
		rejected       bool
	}{
		{"same module", "cache", module + "cache/storages/freecache", false},
		{"declared dependency", "cache", module + "core/errors", false},
		{"transitive dependency", "transport/http/server", module + "config/http", false},
		{"upward dependency", "core/errors", module + "cache", true},
		{"api reaches backend", "factory/cache", module + "cache/storages/redis", true},
		{"api reaches backend factory", "factory/cache", module + "factory/cache/storages/redis", true},
		{"backend factory reaches api", "factory/cache/storages/redis", module + "factory/cache", false},
		{"backend factory reaches backend", "factory/cache/storages/redis", module + "cache/storages/redis", false},
		{"backend factory imports driver library", "factory/cache/storages/redis", "github.com/redis/go-redis/v9", false},
		{"runtime reaches factory", "cache", module + "factory/cache", true},
		{"backend reaches driver base", "cache/storages/redis", module + "internal/redis/redisbase", false},
		{"backend reaches another backend", "cache/storages/redis", module + "reliability/saga/storages/redis", true},
		{"api imports driver library", "cache", "github.com/redis/go-redis/v9", true},
		{"backend imports driver library", "cache/storages/redis", "github.com/redis/go-redis/v9", false},
		{"sdk leaf imports sdk", "security/vault/auth", "github.com/hashicorp/vault/api", false},
		{"api imports sdk", "factory/ext/kryptos", "github.com/hashicorp/vault/api", true},
		{"sdk factory imports sdk", "factory/ext/kryptos/vault", "github.com/hashicorp/vault/api", false},
		{"sdk factory imports its provider", "factory/ext/kryptos/gcp", "github.com/altessa-s/go-kryptos/providers/gcp", false},
		{"neutral factory imports secrets provider", "factory/ext/net/httpclient", "github.com/altessa-s/go-kryptos/providers/gcp", true},
		{"neutral factory imports tls provider", "factory/ext/net/httpclient", "github.com/altessa-s/go-tlsutils/providers/s3", true},
		{"sdk factory imports another provider", "factory/ext/kryptos/gcp", "github.com/altessa-s/go-tlsutils/providers/s3", true},
		{"unmapped package", "devtools", "context", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			reason, _ := p.checkModuleEdge(tc.pkg, tc.dep)
			require.Equal(t, tc.rejected, reason != "", reason)
		})
	}
}

func TestModuleBoundaries(t *testing.T) {
	t.Parallel()
	p, err := newModulePlan()
	require.NoError(t, err)

	seen := map[string]bool{}
	walkProductionImports(t, func(file, pkg, dependency string) {
		reason, edge := p.checkModuleEdge(pkg, dependency)
		if reason == "" {
			return
		}
		key := edge.from + " -> " + edge.to
		if edge.from != "" && moduleViolations[key] {
			seen[key] = true
			return
		}
		t.Errorf("%s imports %s: %s", file, dependency, reason)
	})
	for key := range moduleViolations {
		if !seen[key] {
			t.Errorf("allowlisted module violation %q no longer occurs: remove it from moduleViolations", key)
		}
	}
}
