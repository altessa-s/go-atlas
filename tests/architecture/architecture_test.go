// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package architecture_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const module = "github.com/altessa-s/go-atlas/"

// coreModule is the go-atlas-core module, split out of core/. The rules see
// its packages as core/<path>, the layer they belonged to.
const coreModule = "github.com/altessa-s/go-atlas-core/"

// netModule is the go-atlas-net module, split out of net/. The rules see its
// packages as net/<path>.
const netModule = "github.com/altessa-s/go-atlas-net/"

// nomosModule is the go-nomos module, split out of config/loader and
// config/validation. The rules see its packages as config/<path>.
const nomosModule = "github.com/altessa-s/go-nomos/"

// aipModule is the go-aip module. go-atlas wires its root packages; the
// storage backends are modules of their own that production code never
// imports, so an application requires only the backends it uses.
const aipModule = "github.com/altessa-s/go-aip"

const mongoxModule = "github.com/altessa-s/go-mongox"

// tlsModule is the go-tlsutils module, split out of security/tlsutils. The
// rules see its packages as security/tlsutils/<path>.
const tlsModule = "github.com/altessa-s/go-tlsutils/"

// splitModules are the modules split out of a go-atlas package tree, with
// the path the rules see their root as: go-tlsutils (security/tlsutils),
// go-kryptos (security/secrets), go-slogx (observability/slog) and go-horai
// (service/scheduler, its storage/<backend> packages as storages/<backend>).
var splitModules = []struct{ module, path string }{
	{"github.com/altessa-s/go-tlsutils", "security/tlsutils"},
	{"github.com/altessa-s/go-kryptos", "security/secrets"},
	{"github.com/altessa-s/go-slogx", "observability/slog"},
	{"github.com/altessa-s/go-horai", "service/scheduler"},
}

// internalPath returns the module-relative path of an import of this module,
// of go-atlas-core (as core/<path>), of go-atlas-net (as net/<path>), of
// go-nomos (as config/<path>) or of a split module ([splitModules]), and
// whether it is one.
func internalPath(dependency string) (string, bool) {
	if rest, ok := strings.CutPrefix(dependency, coreModule); ok {
		return "core/" + rest, true
	}
	if rest, ok := strings.CutPrefix(dependency, netModule); ok {
		return "net/" + rest, true
	}
	if rest, ok := strings.CutPrefix(dependency, nomosModule); ok {
		return "config/" + rest, true
	}
	for _, m := range splitModules {
		if dependency == m.module {
			return m.path, true
		}
		if rest, ok := strings.CutPrefix(dependency, m.module+"/"); ok {
			if m.path == "service/scheduler" {
				rest = strings.Replace(rest, "storage/", "storages/", 1)
			}
			return m.path + "/" + rest, true
		}
	}
	return strings.CutPrefix(dependency, module)
}

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
// every package of the factory tree, the helpers shared by factories
// (factory/transport/internal/factoryconv) included.
func isComposition(pkg string) bool {
	return hasPathPrefix(pkg, []string{"factory"})
}

// isTransport reports whether pkg belongs to the transport layer: transport
// and its internals, kept in internal/transport so that the factory tree
// reaches them too.
func isTransport(pkg string) bool {
	return hasPathPrefix(pkg, []string{"transport", "internal/transport"})
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
	dep, internal := internalPath(dependency)
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
	{"runtime packages import no test fixtures", runtimeOffTestFixtures},
	{"transport never depends on services", transportOffServices},
	{"observability factories import no concrete backend", observabilityFactoriesNeutral},
	{"security factories import no remote provider", securityFactoriesNeutral},
	{"storage factories import no ClickHouse", storageFactoriesOffClickHouse},
	{"driver-neutral factories import no driver backend", factoriesOffDrivers},
	{"schemas depend only on core, schemas, config internals and validation libraries", schemasDescribeSettings},
	{"only composition reads schemas", runtimeReadsNoSchemas},
	{"only composition depends on factories", runtimeUsesNoFactories},
	{"config internals never reach transport", configStaysOffTransport},
	{"shared leaf types depend only on core and the standard library", sharedLeavesIsolated},
	{"generic test helpers depend only on core", testhelpersGeneric},
	{"domain depends only on core, domain and observability", domainIsolated},
	{"domain and mapping stay off transport stacks", domainOffTransport},
	{"domain and mapping reach backend translators only from translators", domainOffTranslators},
	{"base observability stays backend-neutral", observabilityNeutral},
	{"production code imports only the root go-aip module", aipRootOnly},
	{"only store/mongo imports go-mongox", mongoxFromMongo},
	{"transport limiters use the limiter contract", limiterOffTokenBucket},
	{"gRPC cache interceptor uses cache contract sentinels", cacheInterceptorOffStorages},
	{"broker and transport use the idempotency contract", idempotencyConsumersOffStorages},
	{"scheduler handler uses injected filter parser", schedulerOffFilterParser},
	{"internal packages import no public capability package", internalOffCapabilities},
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

func runtimeOffTestFixtures(e edge) bool {
	if hasPathPrefix(e.pkg, []string{"internal/testhelpers", "tests"}) || strings.HasSuffix(e.pkg, "test") {
		return false
	}
	return e.dependency == "testing" || strings.HasSuffix(e.dependency, "test") ||
		hasPathPrefix(e.dependency, []string{module + "internal/testhelpers"})
}

// observabilityFactoriesNeutral keeps the metrics factory off concrete
// adapters: a backend registers its builder (UseAdapter), so the factory
// module requires no backend library.
func observabilityFactoriesNeutral(e edge) bool {
	switch e.pkg {
	case "factory/observability/metrics":
		if e.internal {
			return strings.HasPrefix(e.dep, "observability/metrics/adapters/") && e.dep != "observability/metrics/adapters"
		}
		return e.thirdParty && strings.HasPrefix(e.dependency, "github.com/prometheus/")
	case "factory/observability/tracing":
		// The console adapter is built in; every other backend registers.
		return e.internal && (isTransport(e.dep) || strings.HasPrefix(e.dep, "net/") ||
			hasPathPrefix(e.dep, []string{"observability/tracing/adapters/otlp", "factory/observability/tracing/adapters/otlp"}))
	}
	return false
}

// securityFactoriesNeutral keeps the secrets and TLS factories off the remote
// providers, their factories and their SDKs: a provider registers its builder
// (UseProvider) from its own factory package (factory/ext/kryptos/<provider>,
// factory/ext/tlsutils/<provider>), so the factory modules require no cloud, S3
// or Vault library.
func securityFactoriesNeutral(e edge) bool {
	var providers, builtIn []string
	switch e.pkg {
	case "factory/ext/kryptos":
		providers = []string{"security/secrets/providers/", "factory/ext/kryptos/"}
		builtIn = []string{"security/secrets/providers/memory"}
	case "factory/ext/tlsutils":
		providers = []string{"security/tlsutils/providers/", "factory/ext/tlsutils/s3", "factory/ext/tlsutils/vault"}
		builtIn = []string{"security/tlsutils/providers/file", "security/tlsutils/providers/letsencrypt"}
	default:
		return false
	}
	if e.internal {
		return slices.ContainsFunc(providers, func(p string) bool { return strings.HasPrefix(e.dep, p) }) && !slices.Contains(builtIn, e.dep)
	}
	return e.thirdParty
}

// clickHouseFactories are the factories that build a ClickHouse component.
var clickHouseFactories = []string{"factory/infrastructure/clickhouse", "factory/audit/storages/clickhouse", "factory/ext/horai/clickhouse"}

// storageFactoriesOffClickHouse keeps the audit and scheduler factories off
// ClickHouse: its storages register their builders (UseStorage,
// UseHistoryStorage) from their own factory packages.
func storageFactoriesOffClickHouse(e edge) bool {
	if e.pkg != "factory/audit" && e.pkg != "factory/ext/horai" {
		return false
	}
	if e.internal {
		return strings.HasSuffix(e.dep, "/storages/clickhouse") || hasPathPrefix(e.dep, clickHouseFactories)
	}
	return strings.HasPrefix(e.dependency, "github.com/ClickHouse/")
}

// driverNeutralFactories lists the factories that build no driver backend:
// each registers its Redis, MongoDB or NATS backend from that backend's own
// factory package. factory/store/mongo is the MongoDB module's own factory and
// stays off the list.
var driverNeutralFactories = []string{
	"factory/auth/denylist/negcache",
	"factory/auth/oidc",
	"factory/audit",
	"factory/cache",
	"factory/reliability/idempotency",
	"factory/reliability/requestidstore",
	"factory/coord/leadelect",
	"factory/coord/limiters/budget",
	"factory/coord/limiters/tokenbucket",
	"factory/coord/dlock",
	"factory/reliability/outbox",
	"factory/reliability/retryqueue",
	"factory/cache/probfilter",
	"factory/reliability/saga",
	"factory/ext/horai",
	"factory/messaging/broker",
}

// factoriesOffDrivers keeps the driver-neutral factories off driver
// backends and driver libraries.
func factoriesOffDrivers(e edge) bool {
	if !slices.Contains(driverNeutralFactories, e.pkg) {
		return false
	}
	if e.internal {
		for _, driver := range []string{"/redis", "/mongo", "/nats"} {
			if strings.HasSuffix(e.dep, driver) || strings.Contains(e.dep, driver+"/") {
				return true
			}
		}
		return false
	}
	return hasPathPrefix(e.dependency, []string{"github.com/redis", "go.mongodb.org", "github.com/nats-io"})
}

// serviceAdapters maps the transport adapters of split-out services to the
// service they serve: the go-horai scheduler gets its gRPC handler in
// transport/grpc/handlers/scheduler.
var serviceAdapters = map[string]string{"transport/grpc/handlers/scheduler": "service/scheduler"}

// transportOffServices keeps transport and messaging generic: only a service's own
// transport adapter ([serviceAdapters]) reaches that service.
func transportOffServices(e edge) bool {
	if !e.internal || !(isTransport(e.pkg) || strings.HasPrefix(e.pkg, "messaging/")) || !strings.HasPrefix(e.dep, "service/") || hasPathPrefix(e.dep, sharedLeaves) {
		return false
	}
	service, ok := serviceAdapters[e.pkg]
	return !ok || !hasPathPrefix(e.dep, []string{service})
}

func schemasDescribeSettings(e edge) bool {
	if !isSchema(e.pkg) {
		return false
	}
	if !e.internal {
		return e.thirdParty && !allowedSchemaExternal(e.dependency)
	}
	return !strings.HasPrefix(e.dep, "core/") && !isSchema(e.dep) && e.dep != "config/internal" && !strings.HasPrefix(e.dep, "config/internal/") &&
		!(e.pkg == "config/auth" && e.dep == "auth/oidc/rules") &&
		!(e.pkg == "config/proxy" && e.dep == "net/proxyconfig") &&
		e.dep != "net/egressrule" && e.dep != "service/scheduler/task" && e.dep != "service/scheduler/schedule"
}

// runtimeReadsNoSchemas lets config/* (the loader included) read schemas;
// elsewhere only composition does.
func runtimeReadsNoSchemas(e edge) bool {
	return e.internal && isSchema(e.dep) && !isComposition(e.pkg) && !strings.HasPrefix(e.pkg, "config/") && e.pkg != "config"
}

func runtimeUsesNoFactories(e edge) bool {
	return e.internal && isComposition(e.dep) && !isComposition(e.pkg)
}

// sharedLeaves are packages several capabilities import across the area
// graph (reliability/idempotency/principalowner adapts
// principals to reliability/idempotency owner functions; every periodic subsystem
// registers its tasks through the scheduler task contract and parses cron specs with its
// schedule grammar; the LRU caches back net/clientip, parsers, auth and
// secrets alike). They stay leaves so they can become their own modules
// without a module cycle. The scheduler leaves are go-horai packages
// ([splitModules]): the list exempts imports of them, it holds no package of
// this module under those paths.
var sharedLeaves = []string{"auth/principal", "service/scheduler/task", "service/scheduler/schedule", "cache/lru"}

func sharedLeavesIsolated(e edge) bool {
	if !hasPathPrefix(e.pkg, sharedLeaves) {
		return false
	}
	if e.internal {
		return !strings.HasPrefix(e.dep, "core/")
	}
	// The task contract carries the shared cron grammar (ParseSchedule).
	if e.pkg == "service/scheduler/task" && e.dependency == "github.com/robfig/cron/v3" {
		return false
	}
	if e.pkg == "cache/lru" && hasPathPrefix(e.dependency, []string{"github.com/hashicorp/golang-lru/v2"}) {
		return false
	}
	return e.thirdParty
}

// testhelpersGeneric keeps internal/testhelpers free of capability fixtures:
// those live with their capability (audit/audittest, ...), and the
// in-process servers in internal/testhelpers/servers/*.
func testhelpersGeneric(e edge) bool {
	return e.internal && e.pkg == "internal/testhelpers" && !strings.HasPrefix(e.dep, "core/") && !hasPathPrefix(e.dep, sharedLeaves)
}

// internalCapabilityImports lists the packages under the root internal/ that
// import a public capability package of this module, with the packages each
// may import. The list only shrinks: internal packages are shared plumbing
// below the capabilities that import them.
var internalCapabilityImports = map[string][]string{
	"internal/transport/authprincipal": {"auth/principal"},
}

// internalOffCapabilities keeps the root internal/ tree below the public
// packages of this module: it imports other internal packages and external
// modules only, [internalCapabilityImports] aside. internal/testhelpers is
// test support with a rule of its own ([testhelpersGeneric]).
func internalOffCapabilities(e edge) bool {
	if !hasPathPrefix(e.pkg, []string{"internal"}) || hasPathPrefix(e.pkg, []string{"internal/testhelpers"}) {
		return false
	}
	if !strings.HasPrefix(e.dependency, module) || hasPathPrefix(e.dep, []string{"internal"}) {
		return false
	}
	return !hasPathPrefix(e.dep, internalCapabilityImports[e.pkg])
}

func configStaysOffTransport(e edge) bool {
	return e.internal && strings.HasPrefix(e.pkg, "config/") && isTransport(e.dep)
}

func domainIsolated(e edge) bool {
	return e.internal && strings.HasPrefix(e.pkg, "domain/") &&
		!strings.HasPrefix(e.dep, "core/") && !strings.HasPrefix(e.dep, "domain/") && !strings.HasPrefix(e.dep, "observability/")
}

// domainOffTranslators keeps backend translators (mapping/behavior/translators/mongo)
// optional: the rest of domain never imports them, so they can become their
// own driver modules.
func domainOffTranslators(e edge) bool {
	return e.internal && hasPathPrefix(e.pkg, []string{"domain", "mapping"}) && !strings.Contains(e.pkg, "/translators/") &&
		hasPathPrefix(e.dep, []string{"domain", "mapping"}) && strings.Contains(e.dep, "/translators/")
}

// domainTransport lists the transport stacks no domain package may import
// directly. Domain packages reach observability, whose metrics adapters
// interface mentions net/http, so the check is on direct edges only.
var domainTransport = []string{"net/http", "google.golang.org/grpc", module + "transport", module + "net/http", module + "net/grpc"}

func domainOffTransport(e edge) bool {
	return hasPathPrefix(e.pkg, []string{"domain", "mapping"}) && hasPathPrefix(e.dependency, domainTransport)
}

// aipRootOnly keeps production code on the root go-aip module: its storage
// backends (backend/<name>) and test-only modules (integration/...) are
// modules of their own, required by go-atlas tests at most.
func aipRootOnly(e edge) bool {
	return hasPathPrefix(e.dependency, []string{aipModule + "/backend", aipModule + "/integration", aipModule + "/devtools"})
}

func mongoxFromMongo(e edge) bool {
	return hasPathPrefix(e.dependency, []string{mongoxModule}) && !hasPathPrefix(e.pkg, []string{"store/mongo"})
}

// hasPathPrefix reports whether path is one of prefixes or lies below one.
func hasPathPrefix(path string, prefixes []string) bool {
	for _, p := range prefixes {
		if path == p || strings.HasPrefix(path, p+"/") {
			return true
		}
	}
	return false
}

// neutralContracts are the packages docs/architecture.md declares
// backend-neutral. Their whole production dependency set stays free of
// storage and messaging drivers.
var neutralContracts = []string{"observability/tracing", "observability/metrics"}

// drivers lists the storage and messaging client libraries, and this
// module's MongoDB layer, that neutral contracts never reach.
var drivers = []string{
	"go.mongodb.org/mongo-driver",
	"github.com/redis/go-redis",
	"github.com/nats-io/nats.go",
	"github.com/ClickHouse/clickhouse-go",
	"github.com/ClickHouse/ch-go",
	"github.com/jackc/pgx",
	"github.com/lib/pq",
	"github.com/go-sql-driver/mysql",
	"github.com/mattn/go-sqlite3",
	"modernc.org/sqlite",
	module + "store/mongo",
}

// isDriver reports whether the import path is a driver package or one of
// its subpackages. Major-version suffixes count as subpackages.
func isDriver(path string) bool {
	return hasPathPrefix(path, drivers)
}

func observabilityNeutral(e edge) bool {
	if e.pkg != "observability/tracing" && e.pkg != "observability/metrics" {
		return false
	}
	return e.internal &&
		(isTransport(e.dep) || isComposition(e.dep) || strings.HasPrefix(e.dep, e.pkg+"/adapters/"))
}

func TestProductionBoundaries(t *testing.T) {
	t.Parallel()
	walkProductionImports(t, func(file, pkg, dependency string) {
		if v := violation(pkg, dependency); v != "" {
			t.Errorf("%s imports %s: %s; see docs/architecture.md", file, dependency, v)
		}
	})
}

func TestConcreteTypeReferences(t *testing.T) {
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
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		for _, name := range forbiddenConcreteReferences(filepath.ToSlash(rel), f) {
			t.Errorf("%s: %s", path, name)
		}
		return nil
	})
	require.NoError(t, err)
}

type forbiddenConcreteType struct {
	consumer string
	imported string
	typeName string
}

var forbiddenConcreteTypes = []forbiddenConcreteType{
	{"auth/mtls", module + "audit/decision", "Recorder"},
	{"auth/opa", module + "audit/decision", "Recorder"},
	{"transport/grpc/interceptors/auth", module + "audit/decision", "Recorder"},
	{"transport/http/server/middlewares/auth", module + "audit/decision", "Recorder"},
	{"transport/grpc/handlers/health", module + "observability/health", "Coordinator"},
	{"transport/http/server/handlers/health", module + "observability/health", "Coordinator"},
	{"messaging/broker/idempotency", module + "reliability/idempotency", "Keeper"},
	{"transport/grpc/interceptors/audit", module + "audit", "Auditor"},
	{"transport/http/server/middlewares/audit", module + "audit", "Auditor"},
	{"store/mongo", aipModule + "/pagination/pagetoken", "Codec"},
	{"audit", aipModule + "/pagination/pagetoken", "Codec"},
	{"transport/grpc/handlers/scheduler", aipModule + "/pagination/pagetoken", "Codec"},
}

// forbiddenConcreteReferences resolves each selector's package identifier to
// its import path, so aliases cannot bypass the consumer-side seam rule.
func forbiddenConcreteReferences(pkg string, file *ast.File) []string {
	imports := make(map[string]string, len(file.Imports))
	for _, imp := range file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		name := filepath.Base(path)
		if imp.Name != nil {
			name = imp.Name.Name
		}
		imports[name] = path
	}
	var violations []string
	ast.Inspect(file, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		ident, ok := selector.X.(*ast.Ident)
		if !ok {
			return true
		}
		importPath, ok := imports[ident.Name]
		if !ok {
			return true
		}
		for _, rule := range forbiddenConcreteTypes {
			consumerMatch := hasPathPrefix(pkg, []string{rule.consumer})
			if rule.consumer == "store/mongo" || rule.consumer == "audit" {
				consumerMatch = pkg == rule.consumer
			}
			if consumerMatch && importPath == rule.imported && selector.Sel.Name == rule.typeName {
				violations = append(violations, rule.typeName)
			}
		}
		return true
	})
	return violations
}

func TestForbiddenConcreteReferencesRegression(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, source string
		want         bool
	}{
		{"aliased concrete pointer", `package sample; import auditport "github.com/altessa-s/go-atlas/audit/decision"; type S struct { Recorder *auditport.Recorder }`, true},
		{"interface field", `package sample; type Recorder interface { Record() error }; type S struct { Recorder Recorder }`, false},
		{"unrelated selector", `package sample; import other "example.com/other"; type S struct { X *other.Recorder }`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			file, err := parser.ParseFile(token.NewFileSet(), "sample.go", tc.source, 0)
			require.NoError(t, err)
			got := forbiddenConcreteReferences("auth/mtls", file)
			require.Equal(t, tc.want, len(got) != 0)
		})
	}
}

// walkProductionImports calls visit for every import of every non-test Go
// file in the repository, whatever its build constraints, with the file path,
// its package directory relative to the module and the imported path.
func walkProductionImports(t testing.TB, visit func(file, pkg, dependency string)) {
	t.Helper()
	walkImports(t, false, visit)
}

// walkImports can include test binaries and tooling for repository-wide ownership rules.
func walkImports(t testing.TB, includeTests bool, visit func(file, pkg, dependency string)) {
	t.Helper()
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
			case "tests", "devtools":
				if !includeTests {
					return filepath.SkipDir
				}
			case "vendor", "graphify-out":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || (!includeTests && strings.HasSuffix(path, "_test.go")) {
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
			visit(path, filepath.ToSlash(rel), dep)
		}
		return nil
	})
	require.NoError(t, err)
}

var neutralPlatforms = []string{"linux", "darwin", "windows"}

func TestNeutralContractsStayDriverFree(t *testing.T) {
	t.Parallel()
	pkgs := make([]string, len(neutralContracts))
	for i, p := range neutralContracts {
		pkgs[i] = module + p
	}
	for _, goos := range neutralPlatforms {
		t.Run(goos, func(t *testing.T) {
			t.Parallel()
			args := append([]string{"list", "-f", "{{.ImportPath}}{{range .Deps}} {{.}}{{end}}"}, pkgs...)
			cmd := exec.CommandContext(t.Context(), "go", args...)
			cmd.Dir = filepath.Join("..", "..")
			cmd.Env = append(os.Environ(), "GOOS="+goos)
			out, err := cmd.Output()
			require.NoError(t, err, "go list: %s", out)
			for line := range strings.Lines(string(out)) {
				fields := strings.Fields(line)
				for _, dep := range fields[1:] {
					if isDriver(dep) {
						t.Errorf("%s depends on %s (GOOS=%s): backend-neutral contracts stay driver-free; see docs/architecture.md",
							fields[0], dep, goos)
					}
				}
			}
		})
	}
}

func TestIsDriver(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		dep    string
		driver bool
	}{
		{"go.mongodb.org/mongo-driver/v2/bson", true},
		{"go.mongodb.org/mongo-driver/v2/x/bsonx/bsoncore", true},
		{"github.com/redis/go-redis/v9", true},
		{"github.com/nats-io/nats.go/jetstream", true},
		{"github.com/ClickHouse/clickhouse-go/v2", true},
		{"github.com/jackc/pgx/v5/stdlib", true},
		{"github.com/go-sql-driver/mysql", true},
		{module + "store/mongo", true},
		{module + "store/mongo/bsoncodec", true},
		{module + "store/mongodb", false},
		{"github.com/nats-io/nkeys", false},
		{"database/sql/driver", false},
		{"github.com/shopspring/decimal", false},
		{aipModule + "/decimal128", false},
		{"cel.dev/cel-go/cel", false},
	} {
		t.Run(tc.dep, func(t *testing.T) { t.Parallel(); require.Equal(t, tc.driver, isDriver(tc.dep)) })
	}
}

func TestBoundaryRules(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, pkg, dep string
		rejected       bool
	}{
		{"cron grammar exception", "service/scheduler/task", "github.com/robfig/cron/v3", false},
		{"task contract stays a leaf", "service/scheduler/task", module + "cache", true},
		{"transport registers tasks", "messaging/broker/outbox", "github.com/altessa-s/go-horai/task", false},
		{"transport uses scheduler", "messaging/broker/outbox", module + "service/scheduler", true},
		{"lru exception", "cache/lru", "github.com/hashicorp/golang-lru/v2/expirable", false},
		{"lru stays a leaf", "cache/lru", module + "cache", true},
		{"internal uses capability", "internal/redis/redisbase", module + "cache", true},
		{"lease stays below metrics", "internal/nats/natskvlease", module + "observability/metrics", true},
		{"request ID stays below tracing", "internal/transport/requestid", module + "observability/tracing", true},
		{"transport lock classification stays below reliability", "internal/transport/idempotency", module + "reliability/idempotency", true},
		{"internal uses allowlisted capability", "internal/transport/authprincipal", module + "auth/principal", false},
		{"internal allowlist stays narrow", "internal/transport/authprincipal", module + "cache", true},
		{"internal uses internal", "internal/redis/redisbase", module + "internal/redis/redisutils", false},
		{"internal uses core", "internal/redis/redisbase", coreModule + "errors", false},
		{"transport uses service", "transport/grpc/handlers/health", module + "service/scheduler", true},
		{"transport internals use service", "internal/transport/server", module + "service/scheduler", true},
		{"service adapter uses its service", "transport/grpc/handlers/scheduler", "github.com/altessa-s/go-horai", false},
		{"service adapter uses another service", "transport/grpc/handlers/scheduler", module + "service/notifier", true},
		{"metrics factory uses prometheus adapter", "factory/observability/metrics", module + "observability/metrics/adapters/prometheus", true},
		{"metrics factory uses adapter port", "factory/observability/metrics", module + "observability/metrics/adapters", false},
		{"tracing factory uses otlp", "factory/observability/tracing", module + "observability/tracing/adapters/otlp", true},
		{"tracing factory uses otlp factory", "factory/observability/tracing", module + "factory/observability/tracing/adapters/otlp", true},
		{"tracing factory uses transport", "factory/observability/tracing", module + "transport/grpc/client", true},
		{"tracing factory uses console", "factory/observability/tracing", module + "observability/tracing/adapters/console", false},
		{"otlp factory uses transport", "factory/observability/tracing/adapters/otlp", module + "transport/grpc/client", false},
		{"secrets factory uses vault provider", "factory/ext/kryptos", "github.com/altessa-s/go-kryptos/providers/vault", true},
		{"secrets factory uses vault factory", "factory/ext/kryptos", module + "factory/ext/kryptos/vault", true},
		{"secrets factory uses vault sdk", "factory/ext/kryptos", "github.com/hashicorp/vault/api", true},
		{"secrets factory uses memory provider", "factory/ext/kryptos", "github.com/altessa-s/go-kryptos/providers/memory", false},
		{"tls factory uses s3 provider", "factory/ext/tlsutils", tlsModule + "providers/s3", true},
		{"tls factory uses s3 factory", "factory/ext/tlsutils", module + "factory/ext/tlsutils/s3", true},
		{"tls factory uses spiffe factory", "factory/ext/tlsutils", module + "factory/ext/tlsutils/spiffe", false},
		{"tls factory uses aws sdk", "factory/ext/tlsutils", "github.com/aws/aws-sdk-go-v2/service/s3", true},
		{"tls factory uses file provider", "factory/ext/tlsutils", tlsModule + "providers/file", false},
		{"tls factory uses provider port", "factory/ext/tlsutils", tlsModule + "providers", false},
		{"tls factory uses tlsutils root", "factory/ext/tlsutils", "github.com/altessa-s/go-tlsutils", false},
		{"audit factory uses clickhouse storage", "factory/audit", module + "audit/storages/clickhouse", true},
		{"scheduler factory uses clickhouse driver", "factory/ext/horai", "github.com/ClickHouse/clickhouse-go/v2", true},
		{"scheduler factory uses clickhouse factory", "factory/ext/horai", module + "factory/ext/horai/clickhouse", true},
		{"audit factory uses sqldb storage", "factory/audit", module + "audit/storages/sqldb", false},
		{"clickhouse factory uses audit factory", "factory/audit/storages/clickhouse", module + "factory/audit", false},
		{"cache factory uses redis storage", "factory/cache", module + "cache/storages/redis", true},
		{"cache factory uses redis factory", "factory/cache", module + "factory/cache/storages/redis", true},
		{"cache factory uses redis generation", "factory/cache", module + "cache/generation/storages/redis", true},
		{"cache factory uses go-redis", "factory/cache", "github.com/redis/go-redis/v9", true},
		{"saga factory uses nats storage", "factory/reliability/saga", module + "reliability/saga/storages/nats", true},
		{"broker factory uses mongo driver", "factory/messaging/broker", "go.mongodb.org/mongo-driver/v2/mongo", true},
		{"cache factory uses freecache", "factory/cache", module + "cache/storages/freecache", false},
		{"vault factory uses secrets factory", "factory/ext/kryptos/vault", module + "factory/ext/kryptos", false},
		{"prometheus factory uses metrics factory", "factory/observability/metrics/adapters/prometheus", module + "factory/observability/metrics", false},
		{"keyset uses infrastructure factory", "store/mongo/keyset", module + "factory/infrastructure/mongo", true},
		{"schema uses net primitive beyond egress grammar", "config/proxy", module + "net/urlx", true},
		{"domain net/http client", "domain/money", module + "net/http/client", true},
		{"proxy schema uses shared wire schema", "config/proxy", netModule + "proxyconfig", false},
		{"schema uses egress rule grammar", "config/http", module + "net/egressrule", false},
		{"schema transport", "config", module + "net/http/client", true},
		{"schema service", "config", module + "service/scheduler", true},
		{"runtime uses testing", "cache", "testing", true},
		{"runtime uses shared fixtures", "cache", module + "internal/testhelpers", true},
		{"runtime uses nested fixtures", "cache", module + "internal/testhelpers/servers", true},
		{"runtime uses a conformance kit", "cache", module + "audit/storages/storagetest", true},
		{"runtime uses an external fixture", "cache", "net/http/httptest", true},
		{"runtime imports a lookalike", "cache", module + "internal/testhelpersx", false},
		{"kit uses testing", "audit/storages/storagetest", "testing", false},
		{"nested helpers use testing", "internal/testhelpers/servers", "testing", false},
		{"repository tests use kits", "tests/interop", module + "audit/audittest", false},
		{"testhelpers uses data", "internal/testhelpers", module + "audit", true},
		{"testhelpers uses core", "internal/testhelpers", module + "core/scheduler", false},
		{"principal uses third party", "auth/principal", "github.com/google/uuid", true},
		{"principal uses auth", "auth/principal", module + "auth/jwt", true},
		{"domain transport", "domain/money", module + "messaging/broker", true},
		{"domain grpc", "mapping/behavior", "google.golang.org/grpc/metadata", true},
		{"domain grpc root", "mapping/behavior", "google.golang.org/grpc", true},
		{"domain net/http", "domain/money", "net/http", true},
		{"domain net/http subpackage", "domain/money", "net/http/httptest", true},
		{"domain protobuf", "mapping/behavior", "google.golang.org/protobuf/proto", false},
		{"domain genproto", "domain/money/moneypb", "google.golang.org/genproto/googleapis/type/money", false},
		{"domain net/url", "mapping/behavior/translators/mongo", "net/url", false},
		{"domain grpc lookalike", "mapping/behavior", "google.golang.org/grpcx", false},
		{"domain translator driver", "mapping/behavior/translators/mongo", "go.mongodb.org/mongo-driver/v2/bson", false},
		{"domain uses translator", "mapping/behavior", module + "mapping/behavior/translators/mongo", true},
		{"translator uses domain", "mapping/behavior/translators/mongo", module + "mapping/behavior", false},
		{"transport uses grpc", "transport/grpc/server", "google.golang.org/grpc", false},
		{"tracer adapter", "observability/tracing", module + "observability/tracing/adapters/otlp", true},
		{"tracer port", "observability/tracing", module + "observability/tracing/adapters", false},
		{"factory composition", "factory/ext/net/proxydial", module + "config", false},
		{"factory helper reads schema", "factory/transport/internal/factoryconv", module + "config", false},
		{"factory helper uses transport internals", "factory/transport/internal/factoryconv", module + "transport/policy/geoacl", false},
		{"runtime reads schema", "coord/limiters/budget", module + "config", true},
		{"runtime reads schema leaf", "auth/oidc", module + "config/proxy", true},
		{"runtime uses composition", "auth/oidc", module + "factory/auth/oidc", true},
		{"runtime uses factory helper", "transport/grpc/server", module + "factory/transport/internal/factoryconv", true},
		{"transport internals use composition", "internal/transport/server", module + "factory/core", true},
		{"factory uses factory", "factory/auth/oidc", module + "factory/ext/net/proxydial", false},
		{"schema leaf reaches runtime", "config/proxy", module + "net/proxydial", true},
		{"schema leaf uses leaf", "config/grpc", module + "config/tls", false},
		{"schema leaf uses core", "config/tls", module + "core/types/redacted", false},
		{"loader is not a schema", "config/loader/secrets", module + "config/loader/backend", false},
		{"loader reads no schema", "config/loader", module + "config", false},
		{"schema uses validation library", "config/grpc", "github.com/go-ozzo/ozzo-validation/v4/is", false},
		{"schema uses stdlib", "config/tls", "crypto/tls", false},
		{"schema uses driver", "config/mongo", "go.mongodb.org/mongo-driver/v2/mongo", true},
		{"schema uses external client", "config", "github.com/redis/go-redis/v9", true},
		{"factory adapter reads schema", "factory/ext/net/httpclient/observe", module + "config/http", false},
		{"runtime factory lookalike reads schema", "store/foo/factory", module + "config/grpc", true},
		{"runtime factory lookalike uses factory", "store/foo/factory", module + "factory/store/bar", true},
		{"limiter middleware uses tokenbucket", "transport/grpc/interceptors/limiter", module + "coord/limiters/tokenbucket", true},
		{"HTTP limiter middleware uses tokenbucket", "transport/http/server/middlewares/limiter", module + "coord/limiters/tokenbucket", true},
		{"cache interceptor uses storage", "transport/grpc/interceptors/cache", module + "cache/storages", true},
		{"broker uses idempotency storage", "messaging/broker/idempotency", module + "reliability/idempotency/storages", true},
		{"HTTP middleware uses idempotency storage", "transport/http/server/middlewares/idempotency", module + "reliability/idempotency/storages", true},
		{"gRPC interceptor uses idempotency contract", "transport/grpc/interceptors/idempotency", module + "reliability/idempotency", false},
		{"scheduler handler uses parser", "transport/grpc/handlers/scheduler", aipModule + "/filter/parser", true},
		{"scheduler handler uses filter", "transport/grpc/handlers/scheduler", aipModule + "/filter", false},
		{"interceptor uses go-aip update", "transport/grpc/interceptors/fieldmask", aipModule + "/fieldmask/update", false},
		{"keyset uses go-aip backend", "store/mongo/keyset", aipModule + "/backend/mongo", true},
		{"codec uses go-aip SQL kit", "store/mongo/bsoncodec", aipModule + "/backend/sqlbase/sqltype", true},
		{"audit uses go-aip test module", "audit", aipModule + "/integration/docker", true},
		{"go-aip lookalike", "audit", aipModule + "/backendx", false},
		{"mongo adapter uses mongox", "store/mongo/keyset", mongoxModule + "/keyset", false},
		{"other storage uses mongox", "audit/storages/mongo", mongoxModule + "/indexes", true},
		{"mongo path lookalike", "store/mongodb", mongoxModule, true},
		{"mongox module lookalike", "audit", mongoxModule + "-other", false},
	} {
		t.Run(tc.name, func(t *testing.T) { t.Parallel(); require.Equal(t, tc.rejected, violation(tc.pkg, tc.dep) != "") })
	}
}

func TestViolationNamesRule(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ pkg, dep, rule string }{
		{"config/mongo", "go.mongodb.org/mongo-driver/v2/mongo", "schemas depend only on core, schemas, config internals and validation libraries"},
		{"auth/oidc", module + "config/proxy", "only composition reads schemas"},
		{"auth/oidc", module + "factory/auth/oidc", "only composition depends on factories"},
		{"internal/redis/redisbase", module + "cache", "internal packages import no public capability package"},
		{"domain/money", module + "messaging/broker", "domain depends only on core, domain and observability"},
		{"mapping/behavior", "google.golang.org/grpc/metadata", "domain and mapping stay off transport stacks"},
		{"store/mongo/keyset", aipModule + "/backend/mongo", "production code imports only the root go-aip module"},
		{"observability/tracing", module + "observability/tracing/adapters/otlp", "base observability stays backend-neutral"},
	} {
		t.Run(tc.rule, func(t *testing.T) { t.Parallel(); require.Equal(t, tc.rule, violation(tc.pkg, tc.dep)) })
	}
}

func limiterOffTokenBucket(e edge) bool {
	return hasPathPrefix(e.pkg, []string{"transport/grpc/interceptors/limiter", "transport/http/server/middlewares/limiter"}) &&
		e.internal && e.dep == "coord/limiters/tokenbucket"
}

func cacheInterceptorOffStorages(e edge) bool {
	return hasPathPrefix(e.pkg, []string{"transport/grpc/interceptors/cache"}) && e.internal && e.dep == "cache/storages"
}

func idempotencyConsumersOffStorages(e edge) bool {
	return e.internal && e.dep == "reliability/idempotency/storages" && hasPathPrefix(e.pkg, []string{"messaging/broker", "transport"})
}

func schedulerOffFilterParser(e edge) bool {
	return hasPathPrefix(e.pkg, []string{"transport/grpc/handlers/scheduler"}) && e.dependency == aipModule+"/filter/parser"
}
