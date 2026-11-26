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
// root that move into the module when it is split off. relocations maps
// an exact source package to its import path relative to the repository
// after that move. deps lists direct requirements. A module may import any
// module its direct requirements reach.
type targetModule struct {
	root        string
	extra       []string
	relocations map[string]string
	deps        []string
	// drivers lets a non-backend module import driver libraries (the
	// in-process test servers).
	drivers bool
}

// baseModules are the API, schema and runtime modules. They never import a
// driver or heavy SDK; a backend reaches them by registration.
var baseModules = []targetModule{
	{root: "core"},
	{root: "internal/testhelpers", deps: []string{"service/scheduler/task"}},
	{root: "internal/testhelpers/servers", deps: []string{"core"}, drivers: true},
	{root: "internal/sqlident"},
	{root: "internal/sqldialect", deps: []string{"internal/sqlident"}},
	{root: "internal/txscope"},
	{root: "internal/testdbname"},
	{root: "internal/bsontag"},
	{root: "internal/retryqueuefields"},
	{root: "internal/transportconfig"},
	{root: "internal/bsonfield"},
	{root: "internal/purgeloop", deps: []string{"core"}},
	{root: "auth/principal"},
	{root: "auth/oidc/rules", deps: []string{"core", "cache/lru"}},
	{root: "service/scheduler/task"},
	{root: "service/scheduler/schedule"},
	{root: "cache/lru", deps: []string{"core"}},
	{root: "security/secrets/sealbox"},

	{root: "config", deps: []string{
		"core", "net", "service/scheduler/schedule", "internal/sqlident", "internal/retryqueuefields", "internal/transportconfig", "auth/oidc/rules",
	}},
	{root: "observability", extra: []string{"internal/diagnostics"}, deps: []string{"core", "service/scheduler/task"}},
	{root: "observability/metrics/adapters/prometheus", deps: []string{"observability"}},
	{root: "observability/tracing/adapters/otlp", deps: []string{"observability"}},
	{root: "domain", extra: []string{"proto/gen"}, deps: []string{"core"}},
	{root: "cache", deps: []string{"core", "observability", "cache/lru", "service/scheduler/schedule"}},
	{root: "mapping", deps: []string{"core"}},

	{root: "net"},
	{root: "net/grpc"},
	{root: "net/http"},
	{
		root:  "transport",
		extra: []string{"internal/transport"},
		// Rewrite imports of these packages when moving their sources out of
		// the nested gRPC server module. Server-only packages stay in place.
		relocations: map[string]string{
			"transport/grpc/internal/grpcstatus":              "transport/internal/grpcstatus",
			"transport/grpc/interceptors/internal/callstatus": "transport/internal/callstatus",
			"transport/grpc/interceptors":                     "transport/grpcshared/interceptors",
			"transport/grpc/interceptors/defaults":            "transport/grpcshared/interceptors/defaults",
			"transport/grpc/interceptors/driver":              "transport/grpcshared/interceptors/driver",
			"transport/grpc/interceptors/logger":              "transport/grpcshared/interceptors/logger",
			"transport/grpc/interceptors/metadata":            "transport/grpcshared/interceptors/metadata",
			"transport/grpc/interceptors/metrics":             "transport/grpcshared/interceptors/metrics",
			"transport/grpc/interceptors/realip":              "transport/grpcshared/interceptors/realip",
			"transport/grpc/interceptors/requestid":           "transport/grpcshared/interceptors/requestid",
			"transport/grpc/interceptors/tracing":             "transport/grpcshared/interceptors/tracing",
		},
		deps: []string{"core", "observability", "net", "net/grpc", "auth/principal"},
	},
	{root: "transport/grpc/client", deps: []string{"core", "observability", "transport", "net"}},
	{root: "internal/memcleanup", deps: []string{"core", "service/scheduler/task"}},
	{root: "coord", deps: []string{"core", "observability", "cache/lru", "internal/memcleanup", "internal/sqldialect", "internal/purgeloop"}},
	{root: "audit", deps: []string{"core", "observability", "internal/sqldialect", "internal/purgeloop"}},
	{root: "reliability", deps: []string{"core", "observability", "auth/principal", "internal/memcleanup", "internal/sqldialect", "internal/purgeloop"}},
	{root: "auth", deps: []string{"auth/oidc/rules", "core", "observability", "cache", "audit", "auth/principal", "net/http", "service/scheduler/schedule", "net"}},
	{root: "security"},
	{root: "transport/internal/authaudit", deps: []string{"audit"}},
	{root: "transport/http", deps: []string{"transport", "net", "auth", "coord", "reliability", "transport/internal/authaudit", "internal/transportconfig"}},
	{root: "transport/grpc", deps: []string{"transport", "net", "auth", "coord", "reliability", "transport/internal/authaudit"}},
	// The gRPC adapter of the go-horai scheduler stays off the gRPC module.
	{root: "transport/grpc/handlers/scheduler", deps: []string{"service/scheduler", "core"}},
	{root: "messaging/eventbus", deps: []string{"core", "observability", "internal/txscope"}},
	{root: "messaging/broker", deps: []string{"core", "observability", "reliability"}},
	{
		root:  "service/scheduler",
		extra: []string{"proto/gen/scheduler"},
	},

	// factory is the composition layer: the driver-neutral factories, which
	// may import every runtime module but no driver backend or SDK leaf.
	{root: "factory", deps: []string{
		"core", "config", "observability", "cache", "coord", "reliability", "audit", "net", "net/http", "transport",
		"transport/grpc/client", "auth", "security", "transport/http", "transport/grpc", "messaging/broker", "service/scheduler",
		"internal/transportconfig",
	}},
	{root: "factory/observability/metrics/adapters/prometheus", deps: []string{"observability/metrics/adapters/prometheus", "factory"}},
	{root: "factory/observability/tracing/adapters/otlp", deps: []string{"observability/tracing/adapters/otlp", "factory"}},
}

// sdkModules are the leaves carrying a heavy SDK (OPA, Vault, cloud
// secret managers, S3), each factory of one its own leaf next to it.
var sdkModules = []targetModule{
	{root: "store/postgres/migrations", deps: []string{"store/postgres", "internal/sqldialect"}},
	{root: "auth/opa", deps: []string{"core", "audit", "cache/lru", "observability", "service/scheduler/task"}},
	{root: "factory/auth/opa", deps: []string{"auth/opa", "factory"}},
	{root: "security/vault", deps: []string{"core", "observability"}},
	{root: "factory/security/vault", deps: []string{"security/vault", "factory"}},
	{root: "security/secrets/providers/gcp"},
	{root: "factory/ext/kryptos/gcp", deps: []string{"security/secrets/providers/gcp", "factory"}},
	{root: "security/secrets/providers/lockbox"},
	{root: "factory/ext/kryptos/lockbox", deps: []string{"security/secrets/providers/lockbox", "factory"}},
	{root: "security/secrets/providers/vault"},
	{root: "factory/ext/kryptos/vault", deps: []string{"security/secrets/providers/vault", "factory"}},
	{root: "security/tlsutils/providers/s3"},
	{root: "factory/ext/tlsutils/s3", deps: []string{"security/tlsutils/providers/s3", "factory"}},
	{root: "security/tlsutils/providers/vault"},
	{root: "factory/ext/tlsutils/vault", deps: []string{"security/tlsutils/providers/vault", "factory"}},
}

// driverModules are shared driver bases and driver modules with their own dependency sets.
var driverModules = []targetModule{
	{root: "reliability/saga/engines/jetstream", deps: []string{"reliability"}},
	{root: "mapping/behavior/translators/mongo", deps: []string{"mapping", "internal/bsontag", "internal/bsonfield"}},
	{root: "store/mongo", deps: []string{"core", "observability", "internal/memcleanup", "internal/testdbname", "mapping/behavior/translators/mongo"}},
	{root: "store/postgres", deps: []string{"core", "observability", "internal/sqldialect", "internal/txscope"}},
	{root: "store/postgres/postgrestest", deps: []string{"store/postgres", "core", "internal/sqldialect", "internal/testdbname"}},
	{root: "factory/store/postgres", deps: []string{"store/postgres", "factory"}},
	{root: "factory/store/mongo", deps: []string{"store/mongo", "factory"}},
	{root: "store/meilisearch", deps: []string{"core", "net", "net/http"}},
	{root: "internal/redis", deps: []string{"core"}},
	// The RedisBloom plumbing shared by the Bloom and Cuckoo redis backends.
	{root: "cache/probfilter/internal/redisfilter", deps: []string{"core", "cache", "internal/redis"}},
	{root: "internal/nats", deps: []string{"core"}},
	{root: "factory/infrastructure/mongo", deps: []string{"factory", "store/mongo"}},
	{root: "factory/infrastructure/redis", deps: []string{"factory"}},
	{root: "factory/infrastructure/nats", deps: []string{"factory"}},
	{root: "factory/infrastructure/clickhouse", deps: []string{"factory"}},
	{root: "factory/infrastructure/meilisearch", deps: []string{"factory", "store/meilisearch"}},
	{root: "audit/storages/clickhouse", deps: []string{"audit"}},
	{root: "auth/denylist/storages/redis", deps: []string{"internal/redis", "auth", "cache"}},
	{root: "messaging/broker/providers/nats", deps: []string{"messaging/broker"}},
	{root: "reliability/requestidstore/storages/mongo", deps: []string{"store/mongo"}},
	{root: "reliability/requestidstore/storages/redis", deps: []string{"internal/redis"}},
	{root: "reliability/retryqueue/storages/mongo", deps: []string{"store/mongo", "reliability", "internal/retryqueuefields"}},
	{root: "service/scheduler/storages/clickhouse"},
	{root: "service/scheduler/storages/mongo"},
	{root: "service/scheduler/storages/redis"},
}

// driverBackends maps each driver base to the backend modules built on it.
// A backend requires its base and the module of its parent directory.
var driverBackends = map[string][]string{
	"store/mongo": {
		"audit/storages/mongo", "reliability/idempotency/storages/mongo", "coord/dlock/providers/mongo", "reliability/outbox/storages/mongo",
		"reliability/saga/storages/mongo",
	},
	"internal/redis": {
		"cache/storages/redis", "cache/generation/storages/redis",
		"reliability/idempotency/storages/redis", "coord/limiters/storages/redis", "store/mongo/storages/redis",
		"reliability/saga/storages/redis",
	},
	"cache/probfilter/internal/redisfilter": {"cache/probfilter/bloom/storages/redis", "cache/probfilter/cuckoo/storages/redis"},
	"internal/nats": {
		"reliability/idempotency/storages/nats", "coord/leadelect/providers/nats", "coord/limiters/storages/nats",
		"reliability/requestidstore/storages/nats",
		"coord/dlock/providers/nats", "store/mongo/storages/nats",
		"reliability/saga/storages/nats",
	},
}

// backendFactories maps each driver backend with a factory to that factory.
// A backend factory is a leaf of its own: it requires the backend and the
// module of its parent directory, the factory it registers on.
var backendFactories = map[string]string{
	"audit/storages/clickhouse":                 "factory/audit/storages/clickhouse",
	"audit/storages/mongo":                      "factory/audit/storages/mongo",
	"cache/generation/storages/redis":           "factory/cache/generation/storages/redis",
	"cache/storages/redis":                      "factory/cache/storages/redis",
	"reliability/idempotency/storages/mongo":    "factory/reliability/idempotency/storages/mongo",
	"reliability/requestidstore/storages/mongo": "factory/reliability/requestidstore/storages/mongo",
	"reliability/idempotency/storages/nats":     "factory/reliability/idempotency/storages/nats",
	"reliability/requestidstore/storages/nats":  "factory/reliability/requestidstore/storages/nats",
	"reliability/idempotency/storages/redis":    "factory/reliability/idempotency/storages/redis",
	"reliability/requestidstore/storages/redis": "factory/reliability/requestidstore/storages/redis",
	"coord/leadelect/providers/nats":            "factory/coord/leadelect/providers/nats",
	"coord/limiters/storages/nats":              "factory/coord/limiters/storages/nats",
	"coord/limiters/storages/redis":             "factory/coord/limiters/storages/redis",
	"coord/dlock/providers/mongo":               "factory/coord/dlock/providers/mongo",
	"coord/dlock/providers/nats":                "factory/coord/dlock/providers/nats",
	"store/mongo/storages/nats":                 "factory/store/mongo/storages/nats",
	"store/mongo/storages/redis":                "factory/store/mongo/storages/redis",
	"reliability/outbox/storages/mongo":         "factory/reliability/outbox/storages/mongo",
	"reliability/retryqueue/storages/mongo":     "factory/reliability/retryqueue/storages/mongo",
	"cache/probfilter/bloom/storages/redis":     "factory/cache/probfilter/bloom/storages/redis",
	"cache/probfilter/cuckoo/storages/redis":    "factory/cache/probfilter/cuckoo/storages/redis",
	"reliability/saga/storages/mongo":           "factory/reliability/saga/storages/mongo",
	"reliability/saga/storages/nats":            "factory/reliability/saga/storages/nats",
	"reliability/saga/storages/redis":           "factory/reliability/saga/storages/redis",
	"service/scheduler/storages/clickhouse":     "factory/ext/horai/clickhouse",
	"service/scheduler/storages/mongo":          "factory/ext/horai/mongo",
	"service/scheduler/storages/redis":          "factory/ext/horai/redis",
	"messaging/broker/providers/nats":           "factory/messaging/broker/providers/nats",
}

// modulePlan is the resolved target layout.
type modulePlan struct {
	modules map[string]targetModule
	reach   map[string]map[string]bool
}

func newModulePlan() (*modulePlan, error) {
	p := &modulePlan{modules: map[string]targetModule{}}
	for _, set := range [][]targetModule{baseModules, sdkModules, driverModules} {
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

// moduleOf returns the module holding pkg. It prefers an explicit package
// assignment, then the longest matching module root or extra directory.
func (p *modulePlan) moduleOf(pkg string) string {
	best, bestLen := "", -1
	for root, m := range p.modules {
		if _, ok := m.relocations[pkg]; ok {
			return root
		}
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
	if slices.ContainsFunc(driverModules, func(m targetModule) bool { return m.root == root }) ||
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

// checkModuleEdge returns why importing dependency from pkg breaks the
// target layout, or "" when it does not.
func (p *modulePlan) checkModuleEdge(pkg, dependency string) string {
	from := p.moduleOf(pkg)
	if from == "" {
		return "package belongs to no target module"
	}
	if dep, internal := internalPath(dependency); internal {
		to := p.moduleOf(dep)
		if to == "" {
			return "dependency belongs to no target module"
		}
		if !p.allows(from, to) {
			return "module " + from + " does not require " + to
		}
		return ""
	}
	if foreignGooseImport(pkg, dependency) {
		return "goose belongs to store/postgres/migrations"
	}
	if !p.isDriverModule(from) && hasPathPrefix(dependency, driverLibraries) && !redisSchemaURIParser(pkg, dependency) {
		return "module " + from + " imports a driver library"
	}
	return ""
}

func foreignGooseImport(pkg, dependency string) bool {
	return hasPathPrefix(dependency, []string{"github.com/pressly/goose"}) && !hasPathPrefix(pkg, []string{"store/postgres/migrations"})
}

func TestGooseOwnership(t *testing.T) {
	t.Parallel()
	walkImports(t, true, func(file, pkg, dep string) {
		if foreignGooseImport(pkg, dep) {
			t.Errorf("%s imports %s outside store/postgres/migrations", file, dep)
		}
	})
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
		"transport/grpc/interceptors":                    "transport",
		"transport/grpc/interceptors/logger":             "transport",
		"transport/grpc/interceptors/errstatus":          "transport/grpc",
		"transport/grpc/interceptors/health":             "transport/grpc",
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
		{"mongo cannot import goose", "store/mongo", "github.com/pressly/goose/v3/lock", true},
		{"migrations own goose", "store/postgres/migrations", "github.com/pressly/goose/v3", false},
		{"same module", "cache", module + "cache/storages/freecache", false},
		{"declared dependency", "cache", module + "core/errors", false},
		{"store builds documents through the translator", "store/mongo", module + "mapping/behavior/translators/mongo", false},
		{"translator stays independent of the store", "mapping/behavior/translators/mongo", module + "store/mongo", true},
		{"client reaches shared interceptors", "transport/grpc/client", module + "transport/grpc/interceptors/logger", false},
		{"client cannot import server handlers", "transport/grpc/client", module + "transport/grpc/handlers/serviceinfo", true},
		{"client cannot import server-only interceptors", "transport/grpc/client", module + "transport/grpc/interceptors/health", true},
		{"shared interceptors cannot import server-only interceptors", "transport/grpc/interceptors/logger", module + "transport/grpc/interceptors/errstatus", true},
		{"runtime cannot reach schemas", "transport/http/server", module + "config/http", true},
		{"OTLP cannot reach the gRPC client", "observability/tracing/adapters/otlp", module + "transport/grpc/client", true},
		{"config validates SQL identifiers", "config/internal/validators", module + "internal/sqlident", false},
		{"config cannot reach SQL pools", "config/internal/validators", module + "internal/sqldialect", true},
		{"cache uses the schedule grammar", "cache", "github.com/altessa-s/go-horai/schedule", false},
		{"auth uses the schedule grammar", "auth/oidc", "github.com/altessa-s/go-horai/schedule", false},
		{"mongo retry queue uses field names", "reliability/retryqueue/storages/mongo", module + "internal/retryqueuefields", false},
		{"transitive dependency", "transport/http/server", module + "observability/metrics", false},
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
		{"Redis schema uses its URI parser", "config/redis", "github.com/redis/go-redis/v9", false},
		{"other schemas cannot import Redis driver", "config/mongo", "github.com/redis/go-redis/v9", true},
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
			reason := p.checkModuleEdge(tc.pkg, tc.dep)
			require.Equal(t, tc.rejected, reason != "", reason)
		})
	}
}

func TestModuleBoundaries(t *testing.T) {
	t.Parallel()
	p, err := newModulePlan()
	require.NoError(t, err)

	walkProductionImports(t, func(file, pkg, dependency string) {
		reason := p.checkModuleEdge(pkg, dependency)
		if reason != "" {
			t.Errorf("%s imports %s: %s", file, dependency, reason)
		}
	})
}

// moduleEdge connects the planned modules owning the importing and imported packages.
type moduleEdge struct{ from, to string }

func (p *modulePlan) staleEdges(used map[moduleEdge]bool) []moduleEdge {
	var stale []moduleEdge
	for _, root := range slices.Sorted(maps.Keys(p.modules)) {
		for _, dep := range p.modules[root].deps {
			edge := moduleEdge{root, dep}
			if !used[edge] {
				stale = append(stale, edge)
			}
		}
	}
	return stale
}

func TestStaleModuleEdges(t *testing.T) {
	t.Parallel()
	p := modulePlan{modules: map[string]targetModule{
		"a": {deps: []string{"b", "c"}}, "b": {deps: []string{"c"}},
	}}
	used := map[moduleEdge]bool{{"a", "b"}: true, {"b", "c"}: true}
	require.Equal(t, []moduleEdge{{"a", "c"}}, p.staleEdges(used))
}

func TestModuleDependenciesUsed(t *testing.T) {
	t.Parallel()
	p, err := newModulePlan()
	require.NoError(t, err)
	used := map[moduleEdge]bool{}
	walkProductionImports(t, func(_, pkg, dependency string) {
		if dep, ok := internalPath(dependency); ok {
			used[moduleEdge{p.moduleOf(pkg), p.moduleOf(dep)}] = true
		}
	})
	for _, edge := range p.staleEdges(used) {
		t.Errorf("unused module dependency: %s -> %s", edge.from, edge.to)
	}
}
