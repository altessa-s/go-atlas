// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgproto3"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-core/types/redacted"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/observability/health"
	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/observability/metrics/adapters/memory"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	postgresconfig "github.com/altessa-s/go-atlas/config/postgres"
	tlsconfig "github.com/altessa-s/go-atlas/config/tls"
)

func TestBuild_DriverTextNeverReachesErrorsOrLogs(t *testing.T) {
	t.Parallel()
	for _, tc := range testhelpers.PostgresSecretDSNs() {
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			cfg := postgresconfig.Default()
			cfg.DSN = redacted.RedactedString(tc.DSN)
			var logs bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
			_, err := New(&cfg).UseLogger(logger).Build(t.Context())
			require.Error(t, err)
			for _, secret := range tc.Secrets {
				require.False(t, strings.Contains(err.Error(), secret), "password in returned error")
				require.False(t, strings.Contains(logs.String(), secret), "password in lifecycle log")
			}
		})
	}
}

// labelObserver inspects every label emitted by the pool, including the final sample at close.
type labelObserver struct {
	*memory.Adapter
	samples atomic.Int64
	secret  string
	leaked  atomic.Bool
}

func (o *labelObserver) observe(labels map[string]string) {
	o.samples.Add(1)
	for k, v := range labels {
		if strings.Contains(k, o.secret) || strings.Contains(v, o.secret) {
			o.leaked.Store(true)
		}
	}
}
func (o *labelObserver) RecordGauge(_ string, labels map[string]string, _ float64) { o.observe(labels) }
func (o *labelObserver) RecordCounter(_ string, labels map[string]string, _ float64) {
	o.observe(labels)
}
func (o *labelObserver) RecordHistogram(_ string, labels map[string]string, _ float64) {
	o.observe(labels)
}

func TestHealthFailureDoesNotExposeDriverText(t *testing.T) {
	t.Parallel()
	const secret = "s3cret"
	var fail atomic.Bool
	endpoint, _, _ := poolEndpoint(t, func() *pgproto3.ErrorResponse {
		if fail.Load() {
			return &pgproto3.ErrorResponse{Severity: "ERROR", Code: "28000", Message: secret}
		}
		return nil
	})
	cfg := postgresconfig.Default()
	cfg.DSN = redacted.RedactedString(endpoint.Expose() + "&password=" + secret)
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	observed := &labelObserver{Adapter: memory.New(), secret: secret}
	checks := healthRegistry{}
	pools, err := New(&cfg).UseLogger(logger).UseHealthCoordinator(checks).UseMeter(metrics.New(metrics.WithAdapter(observed))).Build(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pools.Close()) })
	require.Equal(t, health.StatusServing, checks["postgres.foreground"].CheckHealth(t.Context()))
	fail.Store(true)
	require.Equal(t, health.StatusNotServing, checks["postgres.foreground"].CheckHealth(t.Context()))
	require.NoError(t, pools.Close())
	require.Positive(t, observed.samples.Load())
	require.False(t, observed.leaked.Load(), "password in metric labels")
	require.True(t, strings.Contains(logs.String(), "PostgreSQL pool opened"), "missing lifecycle log")
	require.False(t, strings.Contains(logs.String(), secret), "password in log")
}

func TestTLSBuildAndDiagnostics(t *testing.T) {
	t.Parallel()
	const secret = "s3cret-password"
	_, _, cert := testhelpers.SelfSignedCert(t)
	roots := x509.NewCertPool()
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[1]})
	require.True(t, roots.AppendCertsFromPEM(caPEM))
	caPath, _ := testhelpers.WriteTempCertFiles(t, caPEM, nil)
	server := &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{cert}}
	for _, tc := range []string{"inherited", "override", "injected", "bad CA", "handshake", "transaction handshake"} {
		t.Run(tc, func(t *testing.T) {
			t.Parallel()
			endpoint, startup, _ := poolEndpointTLS(t, nil, server)
			cfg := postgresconfig.Default()
			cfg.DSN = redacted.RedactedString(endpoint.Expose() + "?password=" + secret)
			cfg.TLS = &tlsconfig.Client{CACerts: []string{caPath}, ServerName: "localhost"}
			p := postgresconfig.Pool{MaxOpenConnections: 1}
			b := New(&cfg)
			switch tc {
			case "override":
				p.TLS = &tlsconfig.Client{CACerts: []string{caPath}, ServerName: "wrong.example"}
			case "injected":
				p.TLS = &tlsconfig.Client{CACerts: []string{"missing"}}
				cfg.TLS.ServerName = "wrong.example"
				b.UseTlsConfig(&tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots, ServerName: "localhost"})
			case "bad CA":
				path := filepath.Join(t.TempDir(), secret)
				require.NoError(t, os.WriteFile(path, []byte("invalid certificate"), 0o600))
				p.TLS = &tlsconfig.Client{CACerts: []string{path}}
			case "transaction handshake":
				cfg.Pooling = "transaction"
				fallthrough
			case "handshake":
				cfg.TLS.ServerName = secret
			}
			cfg.Pools = map[string]postgresconfig.Pool{"worker": p}
			pools, err := b.Build(t.Context())
			switch tc {
			case "inherited", "injected":
				require.NoError(t, err)
				require.NoError(t, pools.Close())
				require.Len(t, startup, 2)
			default:
				require.Error(t, err)
				require.NotContains(t, err.Error(), secret)
				require.NotContains(t, err.Error(), endpoint.Expose())
				if tc == "bad CA" {
					require.ErrorIs(t, err, coreerrs.ErrInvalidArgument)
					require.Empty(t, startup, "a reserved pool's bad CA must fail before opening the primary")
				} else {
					var certErr *tls.CertificateVerificationError
					require.ErrorAs(t, err, &certErr)
				}
			}
		})
	}
}
