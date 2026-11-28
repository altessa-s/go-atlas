// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-horai/scheduler"
	"github.com/altessa-s/go-horai/storage/memory"
	"github.com/altessa-s/go-tlsutils"

	"github.com/altessa-s/go-atlas/factory/ext/tlsutils"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	"golang.org/x/crypto/ocsp"

	tlsconfig "github.com/altessa-s/go-atlas/config/tls"
	schedtask "github.com/altessa-s/go-horai/task"
	tlsocsp "github.com/altessa-s/go-tlsutils/ocsp"
	tlsproviders "github.com/altessa-s/go-tlsutils/providers"
)

var errBuild = errors.New("build failed")

// fakeProvider is a registered provider of a fixed type.
type fakeProvider struct {
	typ tlsproviders.ProviderType
}

func (f fakeProvider) Type() tlsproviders.ProviderType { return f.typ }
func (fakeProvider) TLSConfig() (*tls.Config, error) {
	return &tls.Config{MinVersion: tls.VersionTLS12}, nil
}
func (fakeProvider) Close(context.Context) error { return nil }

// fakeBuilder returns a provider builder of typ that records the environment
// it was given.
func fakeBuilder(typ tlsproviders.ProviderType, env *factory.ProviderEnv) factory.ProviderBuilder {
	return func(_ *tlsconfig.Provider, e factory.ProviderEnv) (tlsproviders.Provider, error) {
		*env = e
		return fakeProvider{typ: typ}, nil
	}
}

type recordingScheduler struct {
	mu  sync.Mutex
	ids []string
}

func (r *recordingScheduler) Register(_ context.Context, cfg schedtask.Config) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ids = append(r.ids, cfg.ID)
	return nil
}

func closeAll(t *testing.T, p *tlsproviders.Providers) {
	t.Helper()
	t.Cleanup(func() {
		p.Close(context.WithoutCancel(t.Context()), func(_ tlsproviders.Provider, err error) {
			t.Errorf("close provider: %v", err)
		})
	})
}

func TestBuild_AllProviders(t *testing.T) {
	t.Parallel()
	certPEM, keyPEM, _ := testhelpers.SelfSignedCert(t)
	certPath, keyPath := testhelpers.WriteTempCertFiles(t, certPEM, keyPEM)
	sched := &recordingScheduler{}
	cacheDir := t.TempDir()

	cfg := &tlsconfig.Provider{
		File:        &tlsconfig.ProviderFile{Certificate: certPath, PrivateKey: keyPath},
		Vault:       &tlsconfig.ProviderVault{CommonName: "svc.example.com", Role: "web"},
		LetsEncrypt: &tlsconfig.ProviderLetsEncrypt{Email: "a@example.com", Domain: []string{"example.com"}},
		S3:          &tlsconfig.ProviderS3{Bucket: "certs", CertificateKey: "tls/cert.pem", PrivateKeyKey: "tls/key.pem"},
		OCSP: &tlsconfig.ProviderOCSP{
			Enabled: true, FailureMode: "soft", EnableCompression: true, MaxCacheEntries: 10,
			HTTPTimeout: 1, RefreshSchedule: "0 */30 * * * *",
		},
	}

	var vaultEnv, s3Env factory.ProviderEnv
	providers, err := factory.New(cfg).
		UseDefaultLogger().
		UseLogger(slog.New(slog.DiscardHandler)).
		UseProvider(tlsproviders.ProviderTypeVault, fakeBuilder(tlsproviders.ProviderTypeVault, &vaultEnv)).
		UseProvider(tlsproviders.ProviderTypeS3, fakeBuilder(tlsproviders.ProviderTypeS3, &s3Env)).
		UseScheduler(sched).
		UseCacheDir(cacheDir).
		Build(t.Context())
	require.NoError(t, err)
	closeAll(t, providers)

	for _, typ := range []tlsproviders.ProviderType{
		tlsproviders.ProviderTypeFile, tlsproviders.ProviderTypeVault,
		tlsproviders.ProviderTypeLetsEncrypt, tlsproviders.ProviderTypeS3,
	} {
		p, ok := providers.Get(typ)
		require.True(t, ok, typ)
		require.Equal(t, typ, p.Type())
	}
	for _, env := range []factory.ProviderEnv{vaultEnv, s3Env} {
		require.NotNil(t, env.Logger)
		require.NotNil(t, env.OCSPStapler, "the config-built stapler is shared with registered providers")
		require.Equal(t, cacheDir, env.CacheDir)
	}
	sched.mu.Lock()
	require.Len(t, sched.ids, 1)
	require.Regexp(t, "^ocsp-stapler-refresh-all-[0-9a-f]{24}$", sched.ids[0])
	sched.mu.Unlock()
}

func TestBuild_ProviderErrors(t *testing.T) {
	t.Parallel()
	missing := filepath.Join(t.TempDir(), "missing.pem")

	_, err := factory.New(&tlsconfig.Provider{File: &tlsconfig.ProviderFile{Certificate: missing, PrivateKey: missing}}).Build(t.Context())
	require.Error(t, err)

	_, err = factory.New(&tlsconfig.Provider{LetsEncrypt: &tlsconfig.ProviderLetsEncrypt{Domain: []string{"example.com"}}}).Build(t.Context())
	require.Error(t, err, "Let's Encrypt needs an email")
}

func TestBuild_UnregisteredProviderFails(t *testing.T) {
	t.Parallel()

	_, err := factory.New(&tlsconfig.Provider{Vault: &tlsconfig.ProviderVault{CommonName: "x", Role: "r"}}).Build(t.Context())
	require.Error(t, err)

	_, err = factory.New(&tlsconfig.Provider{S3: &tlsconfig.ProviderS3{Bucket: "certs"}}).Build(t.Context())
	require.Error(t, err)
}

func TestUseProvider(t *testing.T) {
	t.Parallel()
	cfg := &tlsconfig.Provider{Vault: &tlsconfig.ProviderVault{CommonName: "x", Role: "r"}}

	_, err := factory.New(cfg).UseProvider(tlsproviders.ProviderTypeVault, nil).Build(t.Context())
	require.Error(t, err)

	failing := func(*tlsconfig.Provider, factory.ProviderEnv) (tlsproviders.Provider, error) { return nil, errBuild }
	_, err = factory.New(cfg).UseProvider(tlsproviders.ProviderTypeVault, failing).Build(t.Context())
	require.ErrorIs(t, err, errBuild)
}

func TestCreateClientConfig_ClientCertificateAndCA(t *testing.T) {
	t.Parallel()
	certPEM, keyPEM, _ := testhelpers.SelfSignedCert(t)
	certPath, keyPath := testhelpers.WriteTempCertFiles(t, certPEM, keyPEM)
	caPath := filepath.Join(t.TempDir(), "ca.pem")
	require.NoError(t, os.WriteFile(caPath, certPEM, 0o600))

	cfg, err := factory.CreateClientConfig(&tlsconfig.Client{
		Certificate: certPath, PrivateKey: keyPath, CACerts: []string{caPath}, ServerName: "localhost",
	}, nil)
	require.NoError(t, err)
	require.Len(t, cfg.Certificates, 1)
	require.NotNil(t, cfg.RootCAs)
	require.Equal(t, "localhost", cfg.ServerName)

	_, err = factory.CreateClientConfig(&tlsconfig.Client{CACerts: []string{filepath.Join(t.TempDir(), "x")}}, nil)
	require.Error(t, err)

	_, err = factory.CreateClientConfig(&tlsconfig.Client{Certificate: certPath, PrivateKey: caPath}, nil)
	require.Error(t, err, "a certificate file is not a private key")
}

type ownedProvider struct {
	fakeProvider
	closes      int
	closeErr    error
	closeCtxErr error
}

func (p *ownedProvider) Close(ctx context.Context) error {
	p.closes++
	p.closeCtxErr = ctx.Err()
	return p.closeErr
}

func TestBuildRollsBackBeforeRetry(t *testing.T) {
	t.Parallel()
	closeErr := errors.New("close failed")
	cfg := &tlsconfig.Provider{Vault: &tlsconfig.ProviderVault{}, S3: &tlsconfig.ProviderS3{},
		OCSP: &tlsconfig.ProviderOCSP{Enabled: true, RefreshSchedule: "@hourly"}}
	reg := &testhelpers.MockTaskRegistrar{}
	var opened []*ownedProvider
	b := factory.New(cfg).UseScheduler(reg).UseProvider(tlsproviders.ProviderTypeVault, func(*tlsconfig.Provider, factory.ProviderEnv) (tlsproviders.Provider, error) {
		p := &ownedProvider{typ: tlsproviders.ProviderTypeVault}
		opened = append(opened, p)
		return p, nil
	})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	b.UseProvider(tlsproviders.ProviderTypeS3, func(*tlsconfig.Provider, factory.ProviderEnv) (tlsproviders.Provider, error) {
		opened[0].closeErr = closeErr
		cancel()
		return nil, errBuild
	})
	registry, err := b.Build(ctx)
	require.ErrorIs(t, err, errBuild)
	require.Nil(t, registry)
	require.Equal(t, 1, opened[0].closes)
	require.NoError(t, opened[0].closeCtxErr)
	require.ErrorIs(t, err, closeErr)
	require.Empty(t, reg.Tasks())
	var env factory.ProviderEnv
	registry, err = b.UseProvider(tlsproviders.ProviderTypeS3, fakeBuilder(tlsproviders.ProviderTypeS3, &env)).Build(t.Context())
	require.NoError(t, err)
	require.Len(t, reg.Tasks(), 1)
	require.NoError(t, reg.Tasks()[0].Func(t.Context()))
	require.Zero(t, opened[1].closes)
	registry.Close(t.Context(), nil)
	require.Equal(t, 1, opened[1].closes)
	require.Equal(t, 1, opened[0].closes)
}

type refreshRegistrar struct {
	scheduler *scheduler.Scheduler
	ids       []string
	results   chan error
	contexts  []context.Context
}

func (r *refreshRegistrar) Register(ctx context.Context, cfg schedtask.Config) error {
	r.ids = append(r.ids, cfg.ID)
	r.contexts = append(r.contexts, ctx)
	refresh := cfg.Func
	cfg.Func = func(ctx context.Context) error {
		err := refresh(ctx)
		r.results <- err
		return err
	}
	return r.scheduler.Register(ctx, cfg)
}

// primeStapler makes the cached staple eligible for refresh immediately.
func primeStapler(t testing.TB, stapler tlsutils.OCSPStapler) *atomic.Int32 {
	t.Helper()
	ca := testhelpers.NewCA(t)
	const serial = 5
	now := time.Now()
	response, err := ocsp.CreateResponse(ca.Cert, ca.Cert, ocsp.Response{
		SerialNumber: big.NewInt(serial), Status: ocsp.Good, ThisUpdate: now.Add(-time.Minute), NextUpdate: now.Add(time.Minute),
	}, ca.Key)
	require.NoError(t, err)
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/ocsp-response")
		_, _ = w.Write(response)
	}))
	t.Cleanup(srv.Close)
	cert := ca.SignLeaf(t, testhelpers.WithSerial(serial), testhelpers.WithOCSPServers(srv.URL))
	cert.Certificate = append(cert.Certificate, ca.Cert.Raw)
	got, err := stapler.GetOCSPStaple(t.Context(), &cert)
	require.NoError(t, err)
	require.Equal(t, response, got)
	return &requests
}

func TestIndependentStaplersRefreshOnOneScheduler(t *testing.T) {
	t.Parallel()
	store, err := memory.New()
	require.NoError(t, err)
	sched, err := scheduler.New(store)
	require.NoError(t, err)
	reg := &refreshRegistrar{scheduler: sched, results: make(chan error, 3)}
	var staplers []tlsutils.OCSPStapler
	for _, tc := range []struct{ source, namespace string }{{"first", ""}, {"second", ""}, {"first", "replica"}} {
		cfg := &tlsconfig.Provider{Vault: &tlsconfig.ProviderVault{CommonName: tc.source}, OCSP: &tlsconfig.ProviderOCSP{Enabled: true, RefreshSchedule: "@hourly"}}
		var env factory.ProviderEnv
		registry, err := factory.New(cfg).UseOCSPTaskNamespace(tc.namespace).UseScheduler(reg).
			UseProvider(tlsproviders.ProviderTypeVault, fakeBuilder(tlsproviders.ProviderTypeVault, &env)).Build(t.Context())
		require.NoError(t, err)
		closeAll(t, registry)
		staplers = append(staplers, env.OCSPStapler)
		require.ErrorIs(t, env.OCSPStapler.RunRefreshAll(t.Context()), tlsocsp.ErrSchedulerManaged)
	}
	require.Len(t, reg.ids, 3)
	require.NotEqual(t, reg.ids[0], reg.ids[1])
	require.NotEqual(t, reg.ids[0], reg.ids[2])
	for _, ctx := range reg.contexts {
		require.Equal(t, t.Context().Done(), ctx.Done(), "registration inherits startup cancellation")
	}
	var requests []*atomic.Int32
	for _, stapler := range staplers {
		requests = append(requests, primeStapler(t, stapler))
	}
	require.NoError(t, sched.Start(t.Context()))
	t.Cleanup(func() { require.NoError(t, sched.Stop(context.WithoutCancel(t.Context()))) })
	for i, id := range reg.ids {
		require.NoError(t, sched.Trigger(t.Context(), id))
		select {
		case err := <-reg.results:
			require.NoError(t, err)
			require.EqualValues(t, 2, requests[i].Load(), "the scheduled callback refreshed its own certificate")
		case <-time.After(5 * time.Second):
			require.FailNow(t, "scheduler did not run refresh task", id)
		}
	}
	// A rebuilt logical source retains its ID across policy changes.
	again := &testhelpers.MockTaskRegistrar{}
	cfg := &tlsconfig.Provider{Vault: &tlsconfig.ProviderVault{CommonName: "first"}, OCSP: &tlsconfig.ProviderOCSP{Enabled: true, RefreshSchedule: "@daily"}}
	var env factory.ProviderEnv
	registry, err := factory.New(cfg).UseScheduler(again).UseProvider(tlsproviders.ProviderTypeVault, fakeBuilder(tlsproviders.ProviderTypeVault, &env)).Build(t.Context())
	require.NoError(t, err)
	closeAll(t, registry)
	require.Equal(t, reg.ids[0], again.Tasks()[0].ID)
}

// warmingProvider counts its warm-ups and fails them with err.
type warmingProvider struct {
	fakeProvider
	warmed *int
	err    error
}

func (w warmingProvider) Warm(context.Context) error {
	*w.warmed++
	return w.err
}

// TestBuild_WarmsProviders warms every provider that can obtain its certificates ahead of time and fails the build
// when one cannot.
func TestBuild_WarmsProviders(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		err  error
		fail bool
	}{{nil, false}, {errBuild, true}} {
		warmed := 0
		providers, err := factory.New(&tlsconfig.Provider{Vault: &tlsconfig.ProviderVault{CommonName: "x", Role: "r"}}).
			UseProvider(tlsproviders.ProviderTypeVault, func(*tlsconfig.Provider, factory.ProviderEnv) (tlsproviders.Provider, error) {
				return warmingProvider{typ: tlsproviders.ProviderTypeVault, warmed: &warmed, err: tc.err}, nil
			}).
			Build(t.Context())
		require.Equal(t, 1, warmed)
		if tc.fail {
			require.ErrorIs(t, err, errBuild)
			require.Nil(t, providers)
			continue
		}
		require.NoError(t, err)
		closeAll(t, providers)
	}
}
