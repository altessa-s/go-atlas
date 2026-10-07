// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config_test

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/altessa-s/go-atlas/config/loader"
	"github.com/altessa-s/go-atlas/config/loader/backend/yaml3"

	auditconfig "github.com/altessa-s/go-atlas/config/audit"
	authconfig "github.com/altessa-s/go-atlas/config/auth"
	brokerconfig "github.com/altessa-s/go-atlas/config/broker"
	clienthealthconfig "github.com/altessa-s/go-atlas/config/clienthealth"
	dispatchconfig "github.com/altessa-s/go-atlas/config/dispatch"
	grpcconfig "github.com/altessa-s/go-atlas/config/grpc"
	httpconfig "github.com/altessa-s/go-atlas/config/http"
	idempotencyconfig "github.com/altessa-s/go-atlas/config/idempotency"
	limiterconfig "github.com/altessa-s/go-atlas/config/limiter"
	lockconfig "github.com/altessa-s/go-atlas/config/lock"
	meilisearchconfig "github.com/altessa-s/go-atlas/config/meilisearch"
	mongoconfig "github.com/altessa-s/go-atlas/config/mongo"
	natsconfig "github.com/altessa-s/go-atlas/config/nats"
	nodeconfig "github.com/altessa-s/go-atlas/config/node"
	observabilityconfig "github.com/altessa-s/go-atlas/config/observability"
	pluginsconfig "github.com/altessa-s/go-atlas/config/plugins"
	probfilterconfig "github.com/altessa-s/go-atlas/config/probfilter"
	proxyconfig "github.com/altessa-s/go-atlas/config/proxy"
	redisconfig "github.com/altessa-s/go-atlas/config/redis"
	retryconfig "github.com/altessa-s/go-atlas/config/retry"
	s3config "github.com/altessa-s/go-atlas/config/s3"
	sagaconfig "github.com/altessa-s/go-atlas/config/saga"
	schedulerconfig "github.com/altessa-s/go-atlas/config/scheduler"
	secretsconfig "github.com/altessa-s/go-atlas/config/secrets"
	storageconfig "github.com/altessa-s/go-atlas/config/storage"
	tlsconfig "github.com/altessa-s/go-atlas/config/tls"
	vaultconfig "github.com/altessa-s/go-atlas/config/vault"
	webhookconfig "github.com/altessa-s/go-atlas/config/webhook"
)

// The YAML files under templates/ are the operator-facing reference for the
// structs in this package. They are fully commented out, so nothing in the
// build links them to the code: a renamed field, a changed default, or a new
// knob silently leaves the template stale. The tests below activate every
// template (drop the header, strip one leading '#'), expand its !include
// directives, and check it against the Go type it documents.

const (
	templatesDir = "templates"

	// templateModeLine is the standard header line every template carries.
	templateModeLine = "# All parameters are commented out by default (template mode)."
)

// templateRoot binds one root key of a template to the Go type it documents.
type templateRoot struct {
	key string
	typ reflect.Type
}

func root[T any](key string) templateRoot {
	return templateRoot{key: key, typ: reflect.TypeFor[T]()}
}

// templateCases maps every template file (relative to templatesDir) to its
// root keys and their Go types. A template missing from this table fails
// TestTemplates_EveryFileIsMapped.
var templateCases = map[string][]templateRoot{
	"audit.yaml":                             {root[auditconfig.Config]("audit")},
	"auth.yaml":                              {root[authconfig.Config]("auth")},
	"auth_denylist.yaml":                     {root[authconfig.Denylist]("denylist")},
	"auth_mtls.yaml":                         {root[authconfig.MTLS]("mtls")},
	"auth_oidc.yaml":                         {root[authconfig.OIDC]("oidc")},
	"auth_scope.yaml":                        {root[authconfig.ScopeRegistry]("scope")},
	"broker.yaml":                            {root[brokerconfig.Config]("broker")},
	"cache_storage.yaml":                     {root[storageconfig.CacheStorageConfig]("storage")},
	"dispatch.yaml":                          {root[dispatchconfig.Config]("dispatch")},
	"dlock.yaml":                             {root[lockconfig.DistributionLock]("distributionLock")},
	"grpc.yaml":                              {root[grpcconfig.Config]("grpc")},
	"grpc_proxy.yaml":                        {root[proxyconfig.Config]("proxy")},
	"health.yaml":                            {root[observabilityconfig.Health]("health")},
	"health_client.yaml":                     {root[clienthealthconfig.HTTP]("httpHealthClient"), root[clienthealthconfig.GRPC]("grpcHealthClient")},
	"health_client_grpc.yaml":                {root[clienthealthconfig.GRPC]("grpcHealthClient")},
	"health_client_http.yaml":                {root[clienthealthconfig.HTTP]("httpHealthClient")},
	"http.yaml":                              {root[httpconfig.Config]("http")},
	"http_client_ssrf.yaml":                  {root[httpconfig.ClientSSRF]("ssrf")},
	"http_proxy.yaml":                        {root[proxyconfig.Config]("proxy")},
	"idempotency.yaml":                       {root[idempotencyconfig.Config]("idempotency")},
	"leaderelect.yaml":                       {root[lockconfig.LeaderElector]("leaderElector")},
	"limiter_budget.yaml":                    {root[limiterconfig.Budget]("budgetLimiter")},
	"limiter_tokenbucket.yaml":               {root[limiterconfig.TokenBucket]("tokenBucketLimiter")},
	"logger.yaml":                            {root[observabilityconfig.Logger]("logger")},
	"meilisearch.yaml":                       {root[meilisearchconfig.Config]("meilisearch")},
	"mongo.yaml":                             {root[mongoconfig.Config]("mongodb")},
	"nats.yaml":                              {root[natsconfig.Config]("nats")},
	"node.yaml":                              {root[nodeconfig.Config]("node")},
	"oauth2_client.yaml":                     {root[authconfig.OAuth2Client]("oauth2Client")},
	"observability.yaml":                     {root[observabilityconfig.Config]("observability")},
	"opa.yaml":                               {root[authconfig.OPA]("opa")},
	"plugins.yaml":                           {root[pluginsconfig.Config]("plugins")},
	"probabilistic_filter.yaml":              {root[probfilterconfig.Config]("probabilisticFilter")},
	"redis.yaml":                             {root[redisconfig.Config]("redis")},
	"requests_limiter.yaml":                  {root[limiterconfig.RequestRate]("requestsLimiter")},
	"retry.yaml":                             {root[retryconfig.Config]("retry")},
	"s3.yaml":                                {root[s3config.Config]("s3")},
	"saga.yaml":                              {root[sagaconfig.Config]("saga")},
	"scheduler.yaml":                         {root[schedulerconfig.Config]("scheduler")},
	"secrets.yaml":                           {root[secretsconfig.Config]("secrets")},
	"spiffe.yaml":                            {root[authconfig.SPIFFE]("spiffe")},
	"tls-client.yaml":                        {root[tlsconfig.Client]("tls")},
	"tls-provider.yaml":                      {root[tlsconfig.Provider]("tlsProvider")},
	"vault.yaml":                             {root[vaultconfig.Config]("vault")},
	"wal.yaml":                               {root[dispatchconfig.WAL]("wal")},
	"webhook.yaml":                           {root[webhookconfig.Signature]("webhookSignature")},
	"grpc_interceptors/auth.yaml":            {root[grpcconfig.AuthInterceptor]("auth")},
	"grpc_interceptors/cache.yaml":           {root[grpcconfig.CacheInterceptor]("cache")},
	"grpc_interceptors/errstatus.yaml":       {root[grpcconfig.ErrStatusInterceptor]("errStatus")},
	"grpc_interceptors/geo_acl.yaml":         {root[grpcconfig.GeoACLInterceptor]("geoAcl")},
	"grpc_interceptors/health.yaml":          {root[grpcconfig.HealthInterceptor]("health")},
	"grpc_interceptors/idempotency.yaml":     {root[grpcconfig.IdempotencyInterceptor]("idempotency")},
	"grpc_interceptors/ip_acl.yaml":          {root[grpcconfig.IPACLInterceptor]("ipAcl")},
	"grpc_interceptors/limiter.yaml":         {root[grpcconfig.LimiterInterceptor]("limiter")},
	"grpc_interceptors/logger.yaml":          {root[grpcconfig.LoggerInterceptor]("logger")},
	"grpc_interceptors/metrics.yaml":         {root[grpcconfig.MetricsInterceptor]("metrics")},
	"grpc_interceptors/real_ip.yaml":         {root[grpcconfig.RealIPInterceptor]("realIp")},
	"grpc_interceptors/recovery.yaml":        {root[grpcconfig.RecoveryInterceptor]("recovery")},
	"grpc_interceptors/request_id.yaml":      {root[grpcconfig.RequestIDInterceptor]("requestId")},
	"grpc_interceptors/tracing.yaml":         {root[grpcconfig.TracingInterceptor]("tracing")},
	"http_middlewares/body_limit.yaml":       {root[httpconfig.BodyLimitMiddleware]("bodyLimit")},
	"http_middlewares/cors.yaml":             {root[httpconfig.CORSMiddleware]("cors")},
	"http_middlewares/geo_acl.yaml":          {root[httpconfig.GeoACLMiddleware]("geoAcl")},
	"http_middlewares/idempotency.yaml":      {root[httpconfig.IdempotencyMiddleware]("idempotency")},
	"http_middlewares/ip_acl.yaml":           {root[httpconfig.IPACLMiddleware]("ipAcl")},
	"http_middlewares/limiter.yaml":          {root[httpconfig.LimiterMiddleware]("limiter")},
	"http_middlewares/logger.yaml":           {root[httpconfig.LoggerMiddleware]("logger")},
	"http_middlewares/metrics.yaml":          {root[httpconfig.MetricsMiddleware]("metrics")},
	"http_middlewares/real_ip.yaml":          {root[httpconfig.RealIPMiddleware]("realIp")},
	"http_middlewares/recovery.yaml":         {root[httpconfig.RecoveryMiddleware]("recovery")},
	"http_middlewares/request_id.yaml":       {root[httpconfig.RequestIDMiddleware]("requestId")},
	"http_middlewares/security_headers.yaml": {root[httpconfig.SecurityHeadersMiddleware]("securityHeaders")},
	"http_middlewares/tracing.yaml":          {root[httpconfig.TracingMiddleware]("tracing")},
}

// coverageExceptions lists field-path suffixes (as reported by the coverage
// check) that a template may legitimately leave out, with the reason.
var coverageExceptions = map[string]string{
	"Acl.defaultRule.endpoints": "the default rule applies to every endpoint; factoryconv ignores its endpoint selectors",
	"Acl.defaultRule.patterns":  "the default rule applies to every endpoint; factoryconv ignores its pattern selectors",
}

func isCoverageException(path string) bool {
	for suffix := range coverageExceptions {
		if strings.HasSuffix(path, suffix) {
			return true
		}
	}
	return false
}

func TestTemplates_EveryFileIsMapped(t *testing.T) {
	t.Parallel()

	var onDisk []string
	err := filepath.WalkDir(templatesDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && filepath.Ext(path) == ".yaml" {
			rel, relErr := filepath.Rel(templatesDir, path)
			if relErr != nil {
				return relErr
			}
			onDisk = append(onDisk, filepath.ToSlash(rel))
		}
		return nil
	})
	require.NoError(t, err)

	for _, name := range onDisk {
		_, ok := templateCases[name]
		require.Truef(t, ok, "template %s has no entry in templateCases", name)
	}
	for name := range templateCases {
		require.Containsf(t, onDisk, name, "templateCases maps %s, which does not exist", name)
	}
}

func TestTemplates(t *testing.T) {
	t.Parallel()

	activeDir, activationErrs := activateTemplates(t)

	for name, roots := range templateCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			require.NoError(t, activationErrs[name], "activating the template")

			own, err := os.ReadFile(filepath.Join(activeDir, name))
			require.NoError(t, err)

			expanded, err := (&yaml3.Backend{}).Preprocess(string(own), filepath.Dir(filepath.Join(activeDir, name)), activeDir)
			require.NoError(t, err, "expanding !include directives")

			wrapper := wrapperType(roots)

			t.Run("decodes strictly", func(t *testing.T) {
				t.Parallel()

				dec := yaml.NewDecoder(strings.NewReader(expanded))
				dec.KnownFields(true)
				require.NoError(t, dec.Decode(reflect.New(wrapper).Interface()),
					"the activated template must decode into its Go type with no unknown keys")
			})

			t.Run("documents every field", func(t *testing.T) {
				t.Parallel()

				doc := parseDocument(t, expanded)
				seen := make(map[string]bool)
				for _, r := range roots {
					node := mappingValue(doc, r.key)
					require.NotNilf(t, node, "root %q missing", r.key)
					collectPaths(node, r.typ, r.key, seen)
				}

				var missing []string
				for _, r := range roots {
					for _, p := range fieldPaths(r.typ, r.key, nil) {
						if !seen[p] && !isCoverageException(p) {
							missing = append(missing, p)
						}
					}
				}
				require.Emptyf(t, missing, "fields missing from %s", name)
			})

			t.Run("annotations match the code", func(t *testing.T) {
				t.Parallel()

				// Annotations are checked on the template's own content; each
				// included fragment is checked as a template in its own right.
				doc := parseDocument(t, dropIncludes(string(own)))
				defaults := loaderDefaults(t, wrapper)

				var keys []string
				for i := 0; i+1 < len(doc.Content); i += 2 {
					keys = append(keys, doc.Content[i].Value)
				}
				var want []string
				for _, r := range roots {
					want = append(want, r.key)
				}
				require.ElementsMatch(t, want, keys, "root keys")

				var problems []string
				for i, r := range roots {
					node := mappingValue(doc, r.key)
					checkAnnotations(node, r.typ, defaults.Field(i), r.key, &problems)
				}
				require.Emptyf(t, problems, "annotation problems in %s:\n%s", name, strings.Join(problems, "\n"))
			})
		})
	}
}

// activateTemplates writes the uncommented body of every template into a
// temporary tree mirroring templatesDir, so !include directives resolve to
// activated fragments.
func activateTemplates(t *testing.T) (string, map[string]error) {
	t.Helper()

	dir := t.TempDir()
	errs := make(map[string]error)
	for name := range templateCases {
		raw, err := os.ReadFile(filepath.Join(templatesDir, name))
		require.NoError(t, err)

		body, err := activate(string(raw))
		if err != nil {
			errs[name] = err
			continue
		}

		dst := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(dst), 0o750))
		require.NoError(t, os.WriteFile(dst, []byte(body), 0o600))
	}

	return dir, errs
}

// activate drops the header block and strips one leading '#' from every body
// line. The header is the leading run of blank lines and "# " prose; the body
// starts at the first root line ("#key:").
func activate(raw string) (string, error) {
	lines := strings.Split(raw, "\n")

	start := -1
	for i, line := range lines {
		if line == "" || line == "#" || strings.HasPrefix(line, "# ") {
			continue
		}
		start = i
		break
	}
	if start < 0 {
		return "", fmt.Errorf("no body: expected a commented root line such as \"#key:\"")
	}
	if !slices.Contains(lines[:start], templateModeLine) {
		return "", fmt.Errorf("header lacks the standard line %q", templateModeLine)
	}

	var out strings.Builder
	// Blank out the header so line numbers in failures match the file.
	out.WriteString(strings.Repeat("\n", start))
	for i, line := range lines[start:] {
		switch {
		case strings.TrimSpace(line) == "":
			out.WriteString("\n")
		case strings.HasPrefix(line, "#"):
			out.WriteString(line[1:])
			out.WriteString("\n")
		default:
			return "", fmt.Errorf("line %d is not commented out: %q", start+i+1, line)
		}
	}

	return out.String(), nil
}

var includeLine = regexp.MustCompile(`(?m)^\s*!include\s+.*$\n?`)

func dropIncludes(s string) string { return includeLine.ReplaceAllString(s, "") }

func wrapperType(roots []templateRoot) reflect.Type {
	fields := make([]reflect.StructField, 0, len(roots))
	for i, r := range roots {
		fields = append(fields, reflect.StructField{
			Name: fmt.Sprintf("F%d", i),
			Type: r.typ,
			Tag:  reflect.StructTag(fmt.Sprintf(`yaml:%q`, r.key)),
		})
	}
	return reflect.StructOf(fields)
}

func parseDocument(t *testing.T, s string) *yaml.Node {
	t.Helper()

	var doc yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte(s), &doc))
	require.Equal(t, yaml.DocumentNode, doc.Kind)
	require.Len(t, doc.Content, 1)
	require.Equal(t, yaml.MappingNode, doc.Content[0].Kind)

	return doc.Content[0]
}

func mappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

// yamlField is a struct field as yaml.v3 sees it, inline embeds flattened.
type yamlField struct {
	name  string
	index []int
	typ   reflect.Type
	tag   reflect.StructTag
}

func yamlFields(t reflect.Type) []yamlField {
	var out []yamlField
	for i := range t.NumField() {
		f := t.Field(i)
		tag := f.Tag.Get("yaml")
		if tag == "-" || !f.IsExported() && !f.Anonymous {
			continue
		}
		name, opts, _ := strings.Cut(tag, ",")
		if strings.Contains(opts, "inline") {
			for _, sub := range yamlFields(deref(f.Type)) {
				sub.index = append([]int{i}, sub.index...)
				out = append(out, sub)
			}
			continue
		}
		if name == "" {
			name = strings.ToLower(f.Name)
		}
		out = append(out, yamlField{name: name, index: []int{i}, typ: f.Type, tag: f.Tag})
	}
	return out
}

func deref(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}

var (
	durationType = reflect.TypeFor[time.Duration]()
	timeType     = reflect.TypeFor[time.Time]()
)

// isObject reports whether t is a nested section. time.Time is a struct in Go
// but a scalar timestamp in YAML.
func isObject(t reflect.Type) bool {
	t = deref(t)
	return t.Kind() == reflect.Struct && t != timeType
}

// elemObject returns the struct element type of a slice or map, if any.
func elemObject(t reflect.Type) (reflect.Type, string, bool) {
	t = deref(t)
	switch t.Kind() {
	case reflect.Slice:
		if isObject(t.Elem()) {
			return deref(t.Elem()), "[]", true
		}
	case reflect.Map:
		if isObject(t.Elem()) {
			return deref(t.Elem()), "{}", true
		}
	}
	return nil, "", false
}

// fieldPaths lists the dotted yaml path of every field reachable from t.
func fieldPaths(t reflect.Type, prefix string, visiting []reflect.Type) []string {
	t = deref(t)
	if slices.Contains(visiting, t) {
		return nil
	}
	visiting = append(visiting, t)

	var out []string
	for _, f := range yamlFields(t) {
		p := prefix + "." + f.name
		out = append(out, p)
		switch {
		case isObject(f.typ):
			out = append(out, fieldPaths(f.typ, p, visiting)...)
		default:
			if et, suffix, ok := elemObject(f.typ); ok {
				out = append(out, fieldPaths(et, p+suffix, visiting)...)
			}
		}
	}
	return out
}

// collectPaths records the field paths present in a template node.
func collectPaths(node *yaml.Node, t reflect.Type, prefix string, seen map[string]bool) {
	if node == nil || node.Kind != yaml.MappingNode {
		return
	}
	fields := yamlFields(deref(t))
	for i := 0; i+1 < len(node.Content); i += 2 {
		k, v := node.Content[i], node.Content[i+1]
		idx := slices.IndexFunc(fields, func(f yamlField) bool { return f.name == k.Value })
		if idx < 0 {
			continue // reported by the strict decode
		}
		f := fields[idx]
		p := prefix + "." + f.name
		seen[p] = true
		switch {
		case isObject(f.typ):
			collectPaths(v, f.typ, p, seen)
		default:
			et, suffix, ok := elemObject(f.typ)
			if !ok {
				continue
			}
			if suffix == "[]" && v.Kind == yaml.SequenceNode {
				for _, item := range v.Content {
					collectPaths(item, et, p+suffix, seen)
				}
			}
			if suffix == "{}" && v.Kind == yaml.MappingNode {
				for j := 1; j < len(v.Content); j += 2 {
					collectPaths(v.Content[j], et, p+suffix, seen)
				}
			}
		}
	}
}

// loaderDefaults runs the real config loader (no files, no environment) on a
// skeleton of the wrapper type in which every optional section exists, so the
// value of each field is the default an operator gets once the enclosing
// section is configured.
func loaderDefaults(t *testing.T, wrapper reflect.Type) reflect.Value {
	t.Helper()

	ptr := reflect.New(wrapper)
	allocate(ptr.Elem(), nil)

	_, err := loader.New(nil, loader.WithSkipEnv()).Load(ptr.Interface())
	require.NoError(t, err, "loading defaults")

	return ptr.Elem()
}

// allocate creates every nil struct pointer and gives slices and maps of
// structs a single element, recursively.
func allocate(v reflect.Value, visiting []reflect.Type) {
	switch v.Kind() {
	case reflect.Pointer:
		if !isObject(v.Type().Elem()) {
			return
		}
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		allocate(v.Elem(), visiting)
	case reflect.Struct:
		if !isObject(v.Type()) || slices.Contains(visiting, v.Type()) {
			return
		}
		visiting = append(visiting, v.Type())
		for i := range v.NumField() {
			if v.Type().Field(i).IsExported() && v.Field(i).CanSet() {
				allocate(v.Field(i), visiting)
			}
		}
	case reflect.Slice:
		if v.IsNil() && isObject(v.Type().Elem()) {
			s := reflect.MakeSlice(v.Type(), 1, 1)
			allocate(s.Index(0), visiting)
			v.Set(s)
		}
	case reflect.Map:
		if v.IsNil() && isObject(v.Type().Elem()) {
			m := reflect.MakeMap(v.Type())
			e := reflect.New(v.Type().Elem()).Elem()
			allocate(e, visiting)
			m.SetMapIndex(reflect.New(v.Type().Key()).Elem(), e)
			v.Set(m)
		}
	}
}

var (
	typeAnnotation = regexp.MustCompile(`^#\s*<([^>]*)>`)
	defaultLine    = regexp.MustCompile(`^#\s*Default value:\s*(.*)$`)
	trailingNote   = regexp.MustCompile(`\s+\(.*\)$`)
)

// checkAnnotations verifies the `<type>` and `Default value:` comment lines of
// every key under node against the Go type and the loader default.
func checkAnnotations(node *yaml.Node, t reflect.Type, def reflect.Value, prefix string, problems *[]string) {
	if node == nil || node.Kind != yaml.MappingNode {
		return
	}
	def = derefValue(def)
	fields := yamlFields(deref(t))
	for i := 0; i+1 < len(node.Content); i += 2 {
		k, v := node.Content[i], node.Content[i+1]
		idx := slices.IndexFunc(fields, func(f yamlField) bool { return f.name == k.Value })
		if idx < 0 {
			continue
		}
		f := fields[idx]
		p := prefix + "." + f.name

		comment := k.HeadComment
		if i == 0 && comment == "" {
			comment = node.HeadComment
		}
		typ, defText, problem := parseAnnotation(comment)
		if problem != "" {
			*problems = append(*problems, fmt.Sprintf("%s (line %d): %s", p, k.Line, problem))
		} else if want := typeName(f.typ); typ != want {
			*problems = append(*problems, fmt.Sprintf("%s (line %d): type <%s>, want <%s>", p, k.Line, typ, want))
		}

		if deref(f.typ) == durationType && v.Kind == yaml.ScalarNode && v.Tag != "!!null" && v.Style != yaml.DoubleQuotedStyle {
			*problems = append(*problems, fmt.Sprintf("%s (line %d): duration value %q must be a double-quoted Go duration", p, v.Line, v.Value))
		}

		fv := fieldValue(def, f.index)

		if isObject(f.typ) {
			checkAnnotations(v, f.typ, fv, p, problems)
			continue
		}
		if et, suffix, ok := elemObject(f.typ); ok {
			// Only the first element carries annotations; further elements
			// are examples. The skeleton gave the collection one element, so
			// the collection's own default comes from its tag.
			if suffix == "[]" && v.Kind == yaml.SequenceNode && len(v.Content) > 0 {
				checkAnnotations(v.Content[0], et, firstElem(fv), p+suffix, problems)
			}
			if suffix == "{}" && v.Kind == yaml.MappingNode && len(v.Content) > 1 {
				checkAnnotations(v.Content[1], et, firstElem(fv), p+suffix, problems)
			}
			if tag := f.tag.Get("default"); tag == "" || tag == "-" {
				fv = reflect.Zero(f.typ)
			}
		}

		if problem != "" {
			continue
		}
		if defText == nil {
			*problems = append(*problems, fmt.Sprintf("%s (line %d): no \"Default value:\" line", p, k.Line))
			continue
		}
		if msg := compareDefault(*defText, fv); msg != "" {
			*problems = append(*problems, fmt.Sprintf("%s (line %d): %s", p, k.Line, msg))
		}
	}
}

// parseAnnotation extracts the last `<type>` line of a head comment and the
// `Default value:` line that follows it.
func parseAnnotation(comment string) (typ string, def *string, problem string) {
	lines := strings.Split(comment, "\n")
	start := -1
	for i, line := range lines {
		if m := typeAnnotation.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			typ, start = m[1], i
		}
	}
	if start < 0 {
		return "", nil, "no <type> annotation"
	}
	for _, line := range lines[start+1:] {
		if m := defaultLine.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			if def != nil {
				return "", nil, "more than one \"Default value:\" line"
			}
			s := strings.TrimSpace(m[1])
			def = &s
		}
	}
	return typ, def, ""
}

func derefValue(v reflect.Value) reflect.Value {
	for v.IsValid() && v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return reflect.Value{}
		}
		v = v.Elem()
	}
	return v
}

func fieldValue(v reflect.Value, index []int) reflect.Value {
	if !v.IsValid() {
		return reflect.Value{}
	}
	f, err := v.FieldByIndexErr(index)
	if err != nil {
		return reflect.Value{}
	}
	return f
}

func firstElem(v reflect.Value) reflect.Value {
	v = derefValue(v)
	if !v.IsValid() {
		return reflect.Value{}
	}
	switch v.Kind() {
	case reflect.Slice:
		if v.Len() > 0 {
			return v.Index(0)
		}
	case reflect.Map:
		if iter := v.MapRange(); iter.Next() {
			return iter.Value()
		}
	}
	return reflect.Value{}
}

// typeName renders a Go type in the template notation.
func typeName(t reflect.Type) string {
	t = deref(t)
	switch t {
	case durationType:
		return "duration"
	case timeType:
		return "time"
	}
	switch t.Kind() {
	case reflect.Struct:
		return "object"
	case reflect.Interface:
		return "any"
	case reflect.Slice:
		return "list[" + typeName(t.Elem()) + "]"
	case reflect.Map:
		return "map[" + typeName(t.Key()) + "]" + typeName(t.Elem())
	default:
		return t.Kind().String()
	}
}

func isEmptyValue(v reflect.Value) bool {
	if !v.IsValid() {
		return true
	}
	switch v.Kind() {
	case reflect.Slice, reflect.Map:
		return v.Len() == 0
	default:
		return v.IsZero()
	}
}

// compareDefault checks a documented default against the loader-applied value.
// "not set" (optionally followed by a note) documents a zero loader default;
// anything else is parsed as a YAML value of the field type.
func compareDefault(text string, got reflect.Value) string {
	if strings.HasPrefix(text, "not set") {
		if !isEmptyValue(got) {
			return fmt.Sprintf("documented as %q, but the loader sets %s", text, render(got))
		}
		return ""
	}
	if !got.IsValid() {
		return fmt.Sprintf("documented as %q, but the field is unreachable for the loader", text)
	}
	if got.Kind() == reflect.Pointer && got.IsNil() {
		return fmt.Sprintf("documented as %q, but the loader leaves the pointer unset; write \"not set\"", text)
	}
	got = derefValue(got)

	text = trailingNote.ReplaceAllString(text, "")
	want := reflect.New(got.Type()).Elem()
	if got.Kind() == reflect.String && !strings.HasPrefix(text, `"`) && !strings.HasPrefix(text, `'`) {
		want.SetString(text)
	} else if err := yaml.Unmarshal([]byte(text), want.Addr().Interface()); err != nil {
		return fmt.Sprintf("default %q does not parse as %s: %v", text, got.Type(), err)
	}

	if isEmptyValue(want) && isEmptyValue(got) || reflect.DeepEqual(want.Interface(), got.Interface()) {
		return ""
	}
	return fmt.Sprintf("documented default %q, loader sets %s", text, render(got))
}

func render(v reflect.Value) string {
	v = derefValue(v)
	if !v.IsValid() {
		return "nothing (nil)"
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	if err := enc.Encode(v.Interface()); err != nil {
		return fmt.Sprintf("%v", v.Interface())
	}
	return strings.TrimSpace(buf.String())
}
