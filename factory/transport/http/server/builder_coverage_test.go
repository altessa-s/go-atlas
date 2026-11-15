// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/netip"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/factory/transport/http/server"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/observability/metrics/metricstest"
	"github.com/altessa-s/go-atlas/observability/tracing"
	"github.com/altessa-s/go-atlas/transport/http/server"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares"
	"github.com/altessa-s/go-atlas/transport/http/server/writer"
	"github.com/altessa-s/go-atlas/transport/policy/geoacl"

	httpconfig "github.com/altessa-s/go-atlas/config/http"
	middlewareconfig "github.com/altessa-s/go-atlas/config/middleware"
	tlsconfig "github.com/altessa-s/go-atlas/config/tls"
	sharedlimiter "github.com/altessa-s/go-atlas/coord/limiters"
	corsmw "github.com/altessa-s/go-atlas/transport/http/server/middlewares/cors"
	tlsproviders "github.com/altessa-s/go-tlsutils/providers"
)

// appHandler serves GET /hello and GET /boom (which panics).
type appHandler struct{}

func (appHandler) Register(r server.RouteRegistrar, _ <-chan struct{}) {
	r.Handle("/hello", func(rw writer.ReadWriter) {
		rw.ResponseWriter().WriteHeader(http.StatusOK)
		_, _ = rw.ResponseWriter().Write([]byte("hello"))
	}).Methods(http.MethodGet)
	r.Handle("/boom", func(writer.ReadWriter) { panic("kaboom") }).Methods(http.MethodGet)
}

// start builds b on a loopback port, starts it and returns its base URL.
func start(t *testing.T, b *factory.ServerBuilder) string {
	t.Helper()
	srv, err := b.WithListenAddress("127.0.0.1:0").WithHandler(appHandler{}).Build()
	require.NoError(t, err)
	require.NoError(t, srv.Start())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})
	return "http://" + srv.Listener().Addr().String()
}

func get(t *testing.T, url string, header map[string]string) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	require.NoError(t, err)
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp, string(body)
}

func enable(m *httpconfig.BaseMiddleware) { m.Enabled = true }

type allowAll struct{}

func (allowAll) Limit(context.Context) (*sharedlimiter.LimitInfo, error) {
	return &sharedlimiter.LimitInfo{Limit: 100, Remaining: 99, Reset: time.Now().Add(time.Minute).Unix()}, nil
}

type usGeo struct{}

func (usGeo) Resolve(context.Context, netip.Addr) (geoacl.GeoInfo, error) {
	return geoacl.GeoInfo{CountryCode: "US"}, nil
}

func fullConfig() *httpconfig.Config {
	cfg := httpconfig.Default()
	mws := httpconfig.DefaultMiddlewares()
	for _, m := range []*httpconfig.BaseMiddleware{
		&mws.RealIp.BaseMiddleware, &mws.RequestId.BaseMiddleware, &mws.Recovery.BaseMiddleware,
		&mws.Tracing.BaseMiddleware, &mws.Logger.BaseMiddleware, &mws.Metrics.BaseMiddleware,
		&mws.Cors.BaseMiddleware, &mws.SecurityHeaders.BaseMiddleware, &mws.Limiter.BaseMiddleware,
	} {
		enable(m)
	}
	mws.Cors.AllowedOrigins = []string{"https://app.example.com"}
	mws.Cors.AllowedMethods = []string{http.MethodGet}
	mws.Cors.AllowedHeaders = []string{"X-Token"}
	mws.Cors.ExposedHeaders = []string{"X-Request-ID"}
	mws.Cors.IgnorePaths = []string{"/never"}
	mws.Cors.IgnorePatterns = []string{`^/never/`}
	mws.Cors.AllowCredentials = true
	mws.Cors.AllowPrivateNetwork = true
	mws.Metrics.Subsystem = "web"
	mws.Metrics.EnableSizeMetrics = true
	mws.Metrics.DurationBuckets = []float64{0.1, 1}
	mws.Metrics.SizeBuckets = []float64{10, 100}
	mws.Metrics.IgnorePatterns = []string{`^/never/`}
	mws.Tracing.IgnorePatterns = []string{`^/never/`}
	mws.Logger.IgnorePatterns = []string{`^/never/`}
	mws.Logger.LogRequest = true
	mws.Logger.LogQuery = true
	mws.Logger.RedactQueryParams = []string{"token"}
	mws.Recovery.IgnorePatterns = []string{`^/never/`}
	mws.Limiter.IgnorePatterns = []string{`^/never/`}
	mws.SecurityHeaders.IgnorePatterns = []string{`^/never/`}
	mws.SecurityHeaders.HstsEnabled = true
	mws.SecurityHeaders.HstsPreload = true
	mws.SecurityHeaders.NoDefaultIgnores = true
	mws.IpAcl.Enabled = true
	mws.IpAcl.DefaultPolicy = "deny"
	mws.IpAcl.DefaultRule = &middlewareconfig.IPACLRule{Allowlist: []string{"127.0.0.0/8", "::1/128"}}
	mws.IpAcl.IgnorePatterns = []string{`^/never/`}
	mws.GeoAcl.Enabled = true
	mws.GeoAcl.DefaultPolicy = "deny"
	mws.GeoAcl.DefaultRule = &middlewareconfig.GeoACLRule{AllowCountries: []string{"US"}}
	mws.GeoAcl.IgnorePatterns = []string{`^/never/`}
	cfg.Middlewares = &mws
	return &cfg
}

func TestWithMiddlewares_FullChain(t *testing.T) {
	t.Parallel()

	col := metricstest.NewTestCollector()
	var logs bytes.Buffer
	base := start(t, factory.New(fullConfig()).
		UseLogger(slog.New(slog.NewTextHandler(&logs, nil))).
		UseLogger(nil).
		UseTracer(tracing.Noop()).
		UseMeter(col).
		UseLimiter(allowAll{}).
		UseGeoResolver(usGeo{}).
		WithMiddlewares())

	resp, body := get(t, base+"/hello?token=secret", map[string]string{"Origin": "https://app.example.com"})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "hello", body)
	require.NotEmpty(t, resp.Header.Get("X-Request-ID"), "requestid middleware is installed")
	require.Equal(t, "nosniff", resp.Header.Get("X-Content-Type-Options"), "security headers middleware is installed")
	require.Equal(t, "https://app.example.com", resp.Header.Get("Access-Control-Allow-Origin"), "cors middleware is installed")
	require.Equal(t, "100", resp.Header.Get("X-RateLimit-Limit"), "limiter middleware is installed")
	require.Equal(t, float64(1), metricstest.GetCounterValue(t, col, "test_web_server_requests_total", "method", "GET", "status", "200"))
	require.Contains(t, logs.String(), "/hello", "logger middleware is installed")

	resp, _ = get(t, base+"/boom", nil)
	require.Equal(t, http.StatusInternalServerError, resp.StatusCode, "recovery middleware turns a panic into a 500")

	resp, _ = get(t, base+"/hello", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, "server survives the panic")
}

func TestWithMiddlewares_ExcludeAndDisable(t *testing.T) {
	t.Parallel()

	cfg := fullConfig()
	cfg.Middlewares.GeoAcl.Enabled = false
	base := start(t, factory.New(cfg).
		UseTracer(tracing.Noop()).
		UseLimiter(allowAll{}).
		WithMiddlewares(corsmw.ID).
		WithoutSecurityHeadersMiddleware().
		WithoutLimiterMiddleware())

	resp, _ := get(t, base+"/hello", map[string]string{"Origin": "https://app.example.com"})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Empty(t, resp.Header.Get("Access-Control-Allow-Origin"), "excluded cors")
	require.Empty(t, resp.Header.Get("X-Content-Type-Options"), "disabled security headers")
	require.Empty(t, resp.Header.Get("X-RateLimit-Limit"), "disabled limiter")
}

func TestWithMiddlewares_IPACLDenies(t *testing.T) {
	t.Parallel()

	cfg := httpconfig.Default()
	mws := httpconfig.DefaultMiddlewares()
	enable(&mws.RealIp.BaseMiddleware)
	mws.IpAcl.Enabled = true
	mws.IpAcl.DefaultPolicy = "deny"
	mws.IpAcl.DefaultRule = &middlewareconfig.IPACLRule{Allowlist: []string{"10.0.0.0/8"}}
	cfg.Middlewares = &mws

	base := start(t, factory.New(&cfg).WithMiddlewares())
	resp, _ := get(t, base+"/hello", nil)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestBuild_MissingDependenciesAndBadConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*httpconfig.Middlewares)
	}{
		{name: "tracing without tracer", mutate: func(m *httpconfig.Middlewares) { enable(&m.Tracing.BaseMiddleware) }},
		{name: "limiter without limiter", mutate: func(m *httpconfig.Middlewares) { enable(&m.Limiter.BaseMiddleware) }},
		{name: "geoacl without resolver", mutate: func(m *httpconfig.Middlewares) { m.GeoAcl.Enabled = true }},
		{name: "ipacl with a bad cidr", mutate: func(m *httpconfig.Middlewares) {
			m.IpAcl.Enabled = true
			m.IpAcl.DefaultRule = &middlewareconfig.IPACLRule{Allowlist: []string{"not-a-cidr"}}
		}},
		{name: "realip with a bad proxy", mutate: func(m *httpconfig.Middlewares) {
			enable(&m.RealIp.BaseMiddleware)
			m.RealIp.TrustedProxies = []string{"bogus"}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := httpconfig.Default()
			mws := httpconfig.DefaultMiddlewares()
			tc.mutate(&mws)
			cfg.Middlewares = &mws
			_, err := factory.New(&cfg).WithMiddlewares().Build()
			require.Error(t, err)
		})
	}

	_, err := factory.New(nil).Build()
	require.Error(t, err, "configuration is required")

	srv, err := factory.New(&httpconfig.Config{}).WithMiddlewares().Build()
	require.NoError(t, err, "no middleware section is a no-op")
	require.NotNil(t, srv)

	_, err = factory.New(&httpconfig.Config{Middlewares: &httpconfig.Middlewares{GeoAcl: &httpconfig.GeoACLMiddleware{
		Enabled:       true,
		DefaultPolicy: "bogus",
	}}}).UseGeoResolver(usGeo{}).WithGeoAclMiddleware().Build()
	require.Error(t, err, "invalid geo policy")
}

func TestBuiltinHandlers(t *testing.T) {
	t.Parallel()

	cfg := httpconfig.Default()
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	base := start(t, factory.New(&cfg).WithPprof().UseMetricsHandler(okHandler))
	for _, path := range []string{"/internal/ping", "/internal/healthz", "/internal/readyz", "/internal/pprof/cmdline", "/internal/metrics/"} {
		resp, _ := get(t, base+path, nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, path)
	}

	cfg = httpconfig.Default()
	cfg.Pprof = &httpconfig.Pprof{Enabled: true}
	base = start(t, factory.New(&cfg).WithInternalPrefix("/sys").WithoutMetrics())
	resp, _ := get(t, base+"/sys/ping", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp, _ = get(t, base+"/sys/pprof/cmdline", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, "config enables pprof")
	resp, _ = get(t, base+"/sys/metrics/", nil)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)

	cfg = httpconfig.Default()
	cfg.Pprof = &httpconfig.Pprof{Enabled: true}
	base = start(t, factory.New(&cfg).WithoutPprof())
	resp, _ = get(t, base+"/internal/pprof/cmdline", nil)
	require.Equal(t, http.StatusNotFound, resp.StatusCode, "programmatic override wins over config")

	cfg = httpconfig.Default()
	base = start(t, factory.New(&cfg))
	resp, _ = get(t, base+"/internal/metrics/", nil)
	require.Equal(t, http.StatusNotFound, resp.StatusCode, "no metrics handler, no endpoint")

	cfg = httpconfig.Default()
	base = start(t, factory.New(&cfg).WithoutBuiltinHandlers())
	resp, _ = get(t, base+"/internal/ping", nil)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestRouterPrefixTimeoutsAndCustomMiddleware(t *testing.T) {
	t.Parallel()

	cfg := httpconfig.Default()
	custom := &headerMiddleware{BaseMiddleware: middlewares.NewBaseMiddleware("cov-custom", nil)}
	base := start(t, factory.New(&cfg).
		WithRouterPrefix("/api").
		WithReadTimeout(5*time.Second).
		WithReadHeaderTimeout(2*time.Second).
		WithWriteTimeout(5*time.Second).
		WithIdleTimeout(30*time.Second).
		WithMiddleware(custom).
		WithRawMiddleware(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Raw", "1")
				next.ServeHTTP(w, r)
			})
		}))

	resp, body := get(t, base+"/api/hello", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "hello", body)
	require.Equal(t, "1", resp.Header.Get("X-Custom"))
	require.Equal(t, "1", resp.Header.Get("X-Raw"))

	resp, _ = get(t, base+"/hello", nil)
	require.Equal(t, http.StatusNotFound, resp.StatusCode, "routes live under the prefix")
}

type headerMiddleware struct{ middlewares.BaseMiddleware }

func (*headerMiddleware) Dependencies() []string { return nil }
func (*headerMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom", "1")
		next.ServeHTTP(w, r)
	})
}

type fakeTLS struct {
	cfg *tls.Config
	err error
}

func (fakeTLS) Type() tlsproviders.ProviderType { return tlsproviders.ProviderTypeFile }
func (p fakeTLS) TLSConfig() (*tls.Config, error) {
	if p.err != nil {
		return nil, p.err
	}
	return p.cfg.Clone(), nil
}
func (fakeTLS) Close(context.Context) error { return nil }

func TestBuild_TLS(t *testing.T) {
	t.Parallel()

	_, _, cert := testhelpers.SelfSignedCert(t)
	good := fakeTLS{cfg: &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}}
	providers := func(p tlsproviders.Provider) *tlsproviders.Providers {
		ps := &tlsproviders.Providers{}
		ps.Register(p)
		return ps
	}
	withTLS := func(minVersion string) *httpconfig.Config {
		cfg := httpconfig.Default()
		cfg.TLS = &httpconfig.TLS{Server: &tlsconfig.Server{Provider: tlsconfig.ProviderTypeFile, MinTLSVersion: minVersion}}
		return &cfg
	}

	_, err := factory.New(withTLS("1.2")).Build()
	require.Error(t, err, "tls providers are required")
	_, err = factory.New(withTLS("1.2")).UseTlsProviders(&tlsproviders.Providers{}).Build()
	require.Error(t, err, "provider must be registered")
	_, err = factory.New(withTLS("1.2")).UseTlsProviders(providers(fakeTLS{err: errors.New("no cert")})).Build()
	require.Error(t, err)
	_, err = factory.New(withTLS("0.9")).UseTlsProviders(providers(good)).Build()
	require.Error(t, err)

	srv, err := factory.New(withTLS("1.3")).UseTlsProviders(providers(good)).Build()
	require.NoError(t, err)
	require.Equal(t, uint16(tls.VersionTLS13), srv.TLSConfig().MinVersion)

	srv, err = factory.New(withTLS("1.3")).WithoutTLS().Build()
	require.NoError(t, err)
	require.Nil(t, srv.TLSConfig(), "WithoutTLS ignores configured TLS")

	explicit := &tls.Config{MinVersion: tls.VersionTLS12}
	srv, err = factory.New(withTLS("1.3")).WithTLSConfig(explicit).Build()
	require.NoError(t, err)
	require.Same(t, explicit, srv.TLSConfig())
}
