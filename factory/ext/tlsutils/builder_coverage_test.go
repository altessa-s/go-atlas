// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/factory/ext/tlsutils"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	tlsconfig "github.com/altessa-s/go-atlas/config/tls"
	schedtask "github.com/altessa-s/go-horai/task"
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
		p.Close(context.Background(), func(_ tlsproviders.Provider, err error) {
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
		Build()
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
	require.Equal(t, []string{"ocsp-stapler-refresh-all"}, sched.ids, "the OCSP refresh task is registered")
	sched.mu.Unlock()
}

func TestBuild_ProviderErrors(t *testing.T) {
	t.Parallel()
	missing := filepath.Join(t.TempDir(), "missing.pem")

	_, err := factory.New(&tlsconfig.Provider{File: &tlsconfig.ProviderFile{Certificate: missing, PrivateKey: missing}}).Build()
	require.Error(t, err)

	_, err = factory.New(&tlsconfig.Provider{LetsEncrypt: &tlsconfig.ProviderLetsEncrypt{Domain: []string{"example.com"}}}).Build()
	require.Error(t, err, "Let's Encrypt needs an email")
}

func TestBuild_UnregisteredProviderFails(t *testing.T) {
	t.Parallel()

	_, err := factory.New(&tlsconfig.Provider{Vault: &tlsconfig.ProviderVault{CommonName: "x", Role: "r"}}).Build()
	require.Error(t, err)

	_, err = factory.New(&tlsconfig.Provider{S3: &tlsconfig.ProviderS3{Bucket: "certs"}}).Build()
	require.Error(t, err)
}

func TestUseProvider(t *testing.T) {
	t.Parallel()
	cfg := &tlsconfig.Provider{Vault: &tlsconfig.ProviderVault{CommonName: "x", Role: "r"}}

	_, err := factory.New(cfg).UseProvider(tlsproviders.ProviderTypeVault, nil).Build()
	require.Error(t, err)

	failing := func(*tlsconfig.Provider, factory.ProviderEnv) (tlsproviders.Provider, error) { return nil, errBuild }
	_, err = factory.New(cfg).UseProvider(tlsproviders.ProviderTypeVault, failing).Build()
	require.ErrorIs(t, err, errBuild)
}

func TestCreateClientConfig_ClientCertificateAndCA(t *testing.T) {
	t.Parallel()
	certPEM, keyPEM, _ := testhelpers.SelfSignedCert(t)
	certPath, keyPath := testhelpers.WriteTempCertFiles(t, certPEM, keyPEM)
	caPath := filepath.Join(t.TempDir(), "ca.pem")
	require.NoError(t, os.WriteFile(caPath, certPEM, 0o600))

	cfg, err := factory.New(nil).CreateClientConfig(&tlsconfig.Client{
		Certificate: certPath, PrivateKey: keyPath, CACerts: []string{caPath}, ServerName: "localhost",
	})
	require.NoError(t, err)
	require.Len(t, cfg.Certificates, 1)
	require.NotNil(t, cfg.RootCAs)
	require.Equal(t, "localhost", cfg.ServerName)

	_, err = factory.New(nil).CreateClientConfig(&tlsconfig.Client{CACerts: []string{filepath.Join(t.TempDir(), "x")}})
	require.Error(t, err)

	_, err = factory.New(nil).CreateClientConfig(&tlsconfig.Client{Certificate: certPath, PrivateKey: caPath})
	require.Error(t, err, "a certificate file is not a private key")
}
