// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/factory/transport/grpc/server"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/observability/metrics/metricstest"
	"github.com/altessa-s/go-atlas/observability/tracing"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/cache"
	"github.com/altessa-s/go-atlas/transport/policy/geoacl"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/wrapperspb"

	sharedcache "github.com/altessa-s/go-atlas/cache"
	grpcconfig "github.com/altessa-s/go-atlas/config/grpc"
	middlewareconfig "github.com/altessa-s/go-atlas/config/middleware"
	tlsconfig "github.com/altessa-s/go-atlas/config/tls"
	grpcserver "github.com/altessa-s/go-atlas/transport/grpc/server"
	tlsproviders "github.com/altessa-s/go-tlsutils/providers"
)

const covMethod = "/cov.v1.Echo/Echo"

var on = grpcconfig.BaseInterceptor{Enabled: true}

// echoService registers /cov.v1.Echo/Echo, counting handler calls; a request
// of "panic" panics in the handler.
type echoService struct{ calls atomic.Int32 }

func (e *echoService) Register(gs *grpc.Server, _ <-chan struct{}) {
	gs.RegisterService(&grpc.ServiceDesc{
		ServiceName: "cov.v1.Echo",
		HandlerType: (*any)(nil),
		Methods: []grpc.MethodDesc{{
			MethodName: "Echo",
			Handler: func(srv any, ctx context.Context, dec func(any) error, ic grpc.UnaryServerInterceptor) (any, error) {
				in := &wrapperspb.StringValue{}
				if err := dec(in); err != nil {
					return nil, err
				}
				h := func(_ context.Context, req any) (any, error) {
					e.calls.Add(1)
					if req.(*wrapperspb.StringValue).GetValue() == "panic" {
						panic("handler blew up")
					}
					return req, nil
				}
				if ic == nil {
					return h(ctx, in)
				}
				return ic(ctx, in, &grpc.UnaryServerInfo{Server: srv, FullMethod: covMethod}, h)
			},
		}},
	}, struct{}{})
}

// startBuilt builds b on an in-memory listener, starts it and returns a
// client connection and the echo service.
func startBuilt(t *testing.T, b *factory.ServerBuilder) (*grpc.ClientConn, *echoService) {
	t.Helper()
	svc := &echoService{}
	return startBuiltService(t, b, svc), svc
}

func startBuiltService(t *testing.T, b *factory.ServerBuilder, svc grpcserver.Handler) *grpc.ClientConn {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	srv, err := b.UseServerOptions(grpcserver.WithListener(lis), grpcserver.WithStartupVerification(0)).Build()
	require.NoError(t, err)

	srv.RegisterHandlers(svc)
	require.NoError(t, srv.Start())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func echo(ctx context.Context, conn *grpc.ClientConn, v string) (string, error) {
	out := &wrapperspb.StringValue{}
	err := conn.Invoke(ctx, covMethod, wrapperspb.String(v), out)
	return out.GetValue(), err
}

type unhealthy struct{}

func (unhealthy) Health(context.Context) error { return errors.New("db down") }

func TestBuild_HealthInterceptor(t *testing.T) {
	t.Parallel()

	cfg := &grpcconfig.Config{Interceptors: &grpcconfig.Interceptors{
		Health: &grpcconfig.HealthInterceptor{BaseInterceptor: on},
	}}

	_, err := factory.New(cfg).WithHealthInterceptor().Build()
	require.Error(t, err, "health checker dependency is required")

	conn, svc := startBuilt(t, factory.New(cfg).UseDefaultLogger().UseHealthChecker(unhealthy{}).WithInterceptors())
	_, err = echo(t.Context(), conn, "x")
	require.Equal(t, codes.Unavailable, status.Code(err))
	require.Zero(t, svc.calls.Load())
}

func TestBuild_IPACLDeniesUnknownClient(t *testing.T) {
	t.Parallel()

	cfg := &grpcconfig.Config{Interceptors: &grpcconfig.Interceptors{
		RealIp: &grpcconfig.RealIPInterceptor{BaseInterceptor: on},
		IpAcl: &grpcconfig.IPACLInterceptor{
			BaseInterceptor: on, DefaultPolicy: "deny", FallbackBehavior: "deny",
			Rules: []middlewareconfig.IPACLRule{{Endpoints: []string{covMethod}, Allowlist: []string{"10.0.0.0/8"}}},
		},
	}}
	cfg.Interceptors.IpAcl.IgnorePatterns = []string{`^/never/`}

	conn, svc := startBuilt(t, factory.New(cfg).WithInterceptors())
	_, err := echo(t.Context(), conn, "x")
	require.Equal(t, codes.PermissionDenied, status.Code(err), "a bufconn peer has no client IP and fails closed")
	require.Zero(t, svc.calls.Load())

	bad := &grpcconfig.Config{Interceptors: &grpcconfig.Interceptors{
		IpAcl: &grpcconfig.IPACLInterceptor{
			BaseInterceptor: on, DefaultPolicy: "deny",
			Rules: []middlewareconfig.IPACLRule{{Endpoints: []string{covMethod}, Allowlist: []string{"not-a-cidr"}}},
		},
	}}
	_, err = factory.New(bad).WithIpAclInterceptor().Build()
	require.Error(t, err)
}

type staticGeo struct{}

func (staticGeo) Resolve(context.Context, netip.Addr) (geoacl.GeoInfo, error) {
	return geoacl.GeoInfo{CountryCode: "US"}, nil
}

func TestBuild_GeoACL(t *testing.T) {
	t.Parallel()

	cfg := &grpcconfig.Config{Interceptors: &grpcconfig.Interceptors{
		RealIp: &grpcconfig.RealIPInterceptor{BaseInterceptor: on},
		GeoAcl: &grpcconfig.GeoACLInterceptor{
			BaseInterceptor: on, DefaultPolicy: "deny", FallbackBehavior: "deny",
			Rules: []middlewareconfig.GeoACLRule{{Endpoints: []string{covMethod}, AllowCountries: []string{"US"}}},
		},
	}}
	cfg.Interceptors.GeoAcl.IgnorePatterns = []string{`^/never/`}

	_, err := factory.New(cfg).WithGeoAclInterceptor().Build()
	require.Error(t, err, "geo resolver dependency is required")

	conn, svc := startBuilt(t, factory.New(cfg).UseGeoResolver(staticGeo{}).WithInterceptors())
	_, err = echo(t.Context(), conn, "x")
	require.Equal(t, codes.PermissionDenied, status.Code(err))
	require.Zero(t, svc.calls.Load())

	bad := &grpcconfig.Config{Interceptors: &grpcconfig.Interceptors{
		GeoAcl: &grpcconfig.GeoACLInterceptor{BaseInterceptor: on, DefaultPolicy: "bogus"},
	}}
	_, err = factory.New(bad).UseGeoResolver(staticGeo{}).WithGeoAclInterceptor().Build()
	require.Error(t, err)
}

func TestBuild_MetricsTracingRecovery(t *testing.T) {
	t.Parallel()

	cfg := &grpcconfig.Config{Interceptors: &grpcconfig.Interceptors{
		Metrics: &grpcconfig.MetricsInterceptor{
			BaseInterceptor: on, Subsystem: "edge", EnableSizeMetrics: true, EnableStreamMetrics: true,
			StreamSamplingRate: 1, StreamSamplingStrategy: grpcconfig.MetricsPerMessageSampling,
			DurationBuckets: []float64{0.1, 1}, SizeBuckets: []float64{10, 100},
		},
		Tracing:  &grpcconfig.TracingInterceptor{BaseInterceptor: on},
		Recovery: &grpcconfig.RecoveryInterceptor{BaseInterceptor: on},
	}}
	cfg.Interceptors.Metrics.IgnorePatterns = []string{`^/never/`}
	cfg.Interceptors.Tracing.IgnorePatterns = []string{`^/never/`}
	cfg.Interceptors.Recovery.IgnorePatterns = []string{`^/never/`}

	col := metricstest.NewTestCollector()
	conn, svc := startBuilt(t, factory.New(cfg).UseMeter(col).UseTracer(tracing.Noop()).WithInterceptors())

	got, err := echo(t.Context(), conn, "hello")
	require.NoError(t, err)
	require.Equal(t, "hello", got)
	require.Equal(t, float64(1), metricstest.GetCounterValue(t, col, "test_edge_server_requests_total",
		"method", covMethod, "status", "OK"))

	_, err = echo(t.Context(), conn, "panic")
	require.Error(t, err, "a handler panic becomes an error, not a crash")
	require.NotEqual(t, codes.Unavailable, status.Code(err))

	got, err = echo(t.Context(), conn, "again")
	require.NoError(t, err, "server keeps serving after a recovered panic")
	require.Equal(t, "again", got)
	require.Equal(t, int32(3), svc.calls.Load())
}

// jsonCacher is an in-memory sharedcache.Cacher.
type jsonCacher struct {
	mu sync.Mutex
	m  map[string][]byte
}

func (c *jsonCacher) Save(_ context.Context, key string, v any, _ ...time.Duration) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[key] = b
	return nil
}

func (c *jsonCacher) Get(_ context.Context, key string, v any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	b, ok := c.m[key]
	if !ok {
		return sharedcache.ErrMissing
	}
	return json.Unmarshal(b, v)
}

func (c *jsonCacher) Exists(context.Context, string) (bool, error) { return false, nil }
func (c *jsonCacher) Delete(context.Context, string) error         { return nil }
func (c *jsonCacher) DeleteMany(context.Context, ...string) error  { return nil }
func newJSONCacher() *jsonCacher                                   { return &jsonCacher{m: map[string][]byte{}} }
func (c *jsonCacher) size() int                                    { c.mu.Lock(); defer c.mu.Unlock(); return len(c.m) }
func processor(_ context.Context, md metadata.MD) map[string]string {
	return map[string]string{"t": "x"}
}

func TestBuild_CacheInterceptor(t *testing.T) {
	t.Parallel()

	presets := []grpcconfig.CacheCompressionPreset{
		grpcconfig.CacheCompressionPresetNone, grpcconfig.CacheCompressionPresetFast,
		grpcconfig.CacheCompressionPresetBalanced, grpcconfig.CacheCompressionPresetBest, "unknown",
	}
	for _, preset := range presets {
		t.Run(string(preset), func(t *testing.T) {
			t.Parallel()
			cfg := &grpcconfig.Config{Interceptors: &grpcconfig.Interceptors{
				Cache: &grpcconfig.CacheInterceptor{
					BaseInterceptor: on, SuccessTTL: time.Minute, ErrorTTL: time.Minute,
					CompressionPreset: preset, KeysPrefix: "svc:", KeyMetadata: []string{"x-tenant"},
				},
			}}
			cfg.Interceptors.Cache.IgnorePatterns = []string{`^/never/`}

			_, err := factory.New(cfg).WithCacheInterceptor().Build()
			require.Error(t, err, "cacher dependency is required")

			cacher := newJSONCacher()
			conn, svc := startBuilt(t, factory.New(cfg).
				UseCacher(cacher).
				UseCacheMetadataProcessor(processor).
				UseCacheMethodPolicies(cache.MethodPolicy{Name: covMethod, ResponseProto: &wrapperspb.StringValue{}}).
				WithInterceptors())

			for range 2 {
				got, err := echo(t.Context(), conn, "cached")
				require.NoError(t, err)
				require.Equal(t, "cached", got)
			}
			require.Equal(t, int32(1), svc.calls.Load(), "second call served from cache")
			require.Equal(t, 1, cacher.size())
		})
	}
}

func TestBuild_ReasonCodeOptions(t *testing.T) {
	t.Parallel()

	cfg := &grpcconfig.Config{Interceptors: &grpcconfig.Interceptors{}}
	conn, _ := startBuilt(t, factory.New(cfg).
		UseReasonCode(func(string, string) string { return "X" }).
		UseReasonCodeResolver(nil).
		WithBufValidatorInterceptor())
	got, err := echo(t.Context(), conn, "ok")
	require.NoError(t, err, "messages without validation rules pass")
	require.Equal(t, "ok", got)
}

func TestBuild_WithInterceptorAndGrpcOptions(t *testing.T) {
	t.Parallel()

	timeout, streams, size, buf := 5*time.Second, uint32(64), 1<<20, 32*1024
	cfg := &grpcconfig.Config{
		ConnectionTimeout: &timeout, MaxConcurrentStreams: &streams,
		MaxSendMsgSize: &size, MaxRecvMsgSize: &size, WriteBufferSize: &buf, ReadBufferSize: &buf,
		KeepAlive: &grpcconfig.KeepAlive{Time: time.Minute, Timeout: 10 * time.Second},
	}
	var seen atomic.Int32
	custom := grpcserverInterceptor{fn: func() { seen.Add(1) }}
	conn, _ := startBuilt(t, factory.New(cfg).WithInterceptor(custom))
	_, err := echo(t.Context(), conn, "x")
	require.NoError(t, err)
	require.Equal(t, int32(1), seen.Load(), "a pre-built interceptor joins the chain")
}

// grpcserverInterceptor is a minimal interceptors.ServerInterceptor.
type grpcserverInterceptor struct{ fn func() }

func (grpcserverInterceptor) Name() string { return "cov-custom" }
func (g grpcserverInterceptor) ServerUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, h grpc.UnaryHandler) (any, error) {
		g.fn()
		return h(ctx, req)
	}
}

func (grpcserverInterceptor) ServerStreamInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, h grpc.StreamHandler) error {
		return h(srv, ss)
	}
}

// fakeTLSProvider serves a fixed tls.Config.
type fakeTLSProvider struct {
	cfg *tls.Config
	err error
}

func (p fakeTLSProvider) Type() tlsproviders.ProviderType { return tlsproviders.ProviderTypeFile }
func (p fakeTLSProvider) TLSConfig() (*tls.Config, error) {
	if p.err != nil {
		return nil, p.err
	}
	return p.cfg.Clone(), nil
}
func (fakeTLSProvider) Close(context.Context) error { return nil }

func TestBuild_TLS(t *testing.T) {
	t.Parallel()

	certPEM, keyPEM, cert := testhelpers.SelfSignedCert(t)
	caPath, _ := testhelpers.WriteTempCertFiles(t, certPEM, keyPEM)

	withTLS := func(minVersion string, client *tlsconfig.ClientAuth) *grpcconfig.Config {
		return &grpcconfig.Config{TLS: &grpcconfig.TLS{
			Server: &tlsconfig.Server{Provider: tlsconfig.ProviderTypeFile, MinTLSVersion: minVersion},
			Client: client,
		}}
	}
	providers := func(p tlsproviders.Provider) *tlsproviders.Providers {
		ps := &tlsproviders.Providers{}
		ps.Register(p)
		return ps
	}
	good := fakeTLSProvider{cfg: &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}}

	_, err := factory.New(withTLS("1.2", nil)).Build()
	require.Error(t, err, "tls providers are required")

	_, err = factory.New(withTLS("1.2", nil)).UseTlsProviders(&tlsproviders.Providers{}).Build()
	require.Error(t, err, "configured provider must be registered")

	_, err = factory.New(withTLS("1.2", nil)).UseTlsProviders(providers(fakeTLSProvider{err: errors.New("no cert")})).Build()
	require.Error(t, err)

	_, err = factory.New(withTLS("1.1", nil)).UseTlsProviders(providers(good)).Build()
	require.Error(t, err, "unsupported minimum version")

	_, err = factory.New(withTLS("1.2", &tlsconfig.ClientAuth{AllowCertAuth: true, CACerts: []string{"/does/not/exist.pem"}})).
		UseTlsProviders(providers(good)).Build()
	require.Error(t, err, "unreadable client CA")

	srv, err := factory.New(withTLS("1.3", &tlsconfig.ClientAuth{AllowCertAuth: true, CACerts: []string{caPath}})).
		UseTlsProviders(providers(good)).Build()
	require.NoError(t, err)
	tc := srv.TLSConfig()
	require.NotNil(t, tc)
	require.Equal(t, uint16(tls.VersionTLS13), tc.MinVersion)
	require.Equal(t, tls.RequireAndVerifyClientCert, tc.ClientAuth)
	require.NotNil(t, tc.ClientCAs)

	srv, err = factory.New(withTLS("1.2", &tlsconfig.ClientAuth{AllowCertAuth: false, CACerts: []string{caPath}})).
		UseTlsProviders(providers(good)).Build()
	require.NoError(t, err)
	require.Equal(t, tls.NoClientCert, srv.TLSConfig().ClientAuth, "client CAs are ignored unless cert auth is allowed")
}
