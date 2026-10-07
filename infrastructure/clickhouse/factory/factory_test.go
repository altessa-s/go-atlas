// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"crypto/tls"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/types/redacted"

	chgo "github.com/ClickHouse/clickhouse-go/v2"
	clickhouseconfig "github.com/altessa-s/go-atlas/config/clickhouse"
	tlsconfig "github.com/altessa-s/go-atlas/config/tls"
	chfactory "github.com/altessa-s/go-atlas/infrastructure/clickhouse/factory"
)

func testConfig() *clickhouseconfig.Config {
	cfg := clickhouseconfig.Default()
	cfg.Hosts = []string{"ch-1:9000", "ch-2:9000"}
	cfg.Database = "analytics"
	cfg.Username = "writer"
	cfg.Password = redacted.RedactedString("s3cret")

	return &cfg
}

func TestBuildRequiresConfig(t *testing.T) {
	t.Parallel()

	conn, err := chfactory.New(nil).Build(t.Context())

	require.Nil(t, conn)
	require.Error(t, err)
}

// The driver dials lazily, so Build succeeds without a server.
func TestBuildOpensConnection(t *testing.T) {
	t.Parallel()

	conn, err := chfactory.New(testConfig()).Build(t.Context())

	require.NoError(t, err)
	require.NotNil(t, conn)

	t.Cleanup(func() { _ = conn.Close() })
}

func TestClientOptionsFromFields(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	cfg.DialTimeout = 3 * time.Second
	cfg.ReadTimeout = 15 * time.Second
	cfg.MaxOpenConns = 20
	cfg.MaxIdleConns = 7
	cfg.ConnMaxLifetime = 2 * time.Hour
	cfg.Settings = map[string]string{"max_execution_time": "60"}

	opts, err := chfactory.New(cfg).ClientOptions()
	require.NoError(t, err)

	require.Equal(t, []string{"ch-1:9000", "ch-2:9000"}, opts.Addr)
	require.Equal(t, "analytics", opts.Auth.Database)
	require.Equal(t, "writer", opts.Auth.Username)
	require.Equal(t, "s3cret", opts.Auth.Password)
	require.Equal(t, 3*time.Second, opts.DialTimeout)
	require.Equal(t, 15*time.Second, opts.ReadTimeout)
	require.Equal(t, 20, opts.MaxOpenConns)
	require.Equal(t, 7, opts.MaxIdleConns)
	require.Equal(t, 2*time.Hour, opts.ConnMaxLifetime)
	require.Equal(t, chgo.Settings{"max_execution_time": "60"}, opts.Settings)
	require.Nil(t, opts.TLS)
}

func TestClientOptionsCompression(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		value      clickhouseconfig.Compression
		wantMethod chgo.CompressionMethod
		wantSet    bool
	}{
		{name: "lz4", value: clickhouseconfig.CompressionLZ4, wantMethod: chgo.CompressionLZ4, wantSet: true},
		{name: "zstd", value: clickhouseconfig.CompressionZSTD, wantMethod: chgo.CompressionZSTD, wantSet: true},
		{name: "none", value: clickhouseconfig.CompressionNone, wantMethod: chgo.CompressionNone, wantSet: true},
		{name: "gzip", value: clickhouseconfig.CompressionGZIP, wantMethod: chgo.CompressionGZIP, wantSet: true},
		{name: "unset leaves driver default", value: ""},
		{name: "unknown leaves driver default", value: "bogus"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := testConfig()
			cfg.Compression = tt.value

			opts, err := chfactory.New(cfg).ClientOptions()
			require.NoError(t, err)

			if !tt.wantSet {
				require.Nil(t, opts.Compression)

				return
			}

			require.NotNil(t, opts.Compression)
			require.Equal(t, tt.wantMethod, opts.Compression.Method)
		})
	}
}

func TestClientOptionsAppliesTLS(t *testing.T) {
	t.Parallel()

	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS13}

	opts, err := chfactory.New(testConfig()).UseTlsConfig(tlsCfg).ClientOptions()
	require.NoError(t, err)

	require.Same(t, tlsCfg, opts.TLS)
}

func TestClientOptionsFromConnectionURI(t *testing.T) {
	t.Parallel()

	cfg := clickhouseconfig.Default()
	cfg.Username = ""
	cfg.ConnectionURI = redacted.RedactedString("clickhouse://dsnuser:dsnpass@dsn-host:9000/dsndb")
	cfg.MaxOpenConns = 42
	cfg.Settings = map[string]string{"max_block_size": "8192"}

	opts, err := chfactory.New(&cfg).ClientOptions()
	require.NoError(t, err)

	// The DSN wins for the connection target...
	require.Equal(t, []string{"dsn-host:9000"}, opts.Addr)
	require.Equal(t, "dsndb", opts.Auth.Database)
	require.Equal(t, "dsnuser", opts.Auth.Username)

	// ...while the remaining config is layered on top.
	require.Equal(t, 42, opts.MaxOpenConns)
	require.Equal(t, chgo.Settings{"max_block_size": "8192"}, opts.Settings)
}

func TestClientOptionsRejectsMalformedConnectionURI(t *testing.T) {
	t.Parallel()

	cfg := clickhouseconfig.Default()
	cfg.Username = ""
	cfg.ConnectionURI = redacted.RedactedString("://not-a-dsn")

	opts, err := chfactory.New(&cfg).ClientOptions()

	require.Nil(t, opts)
	require.Error(t, err)
}

func TestConfigValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(*clickhouseconfig.Config)
		wantErr bool
	}{
		{name: "defaults are valid", mutate: func(*clickhouseconfig.Config) {}},
		{
			name:    "hosts required",
			mutate:  func(c *clickhouseconfig.Config) { c.Hosts = nil },
			wantErr: true,
		},
		{
			name:    "database required",
			mutate:  func(c *clickhouseconfig.Config) { c.Database = "" },
			wantErr: true,
		},
		{
			name:    "max open conns must be positive",
			mutate:  func(c *clickhouseconfig.Config) { c.MaxOpenConns = 0 },
			wantErr: true,
		},
		{
			name:    "unknown compression rejected",
			mutate:  func(c *clickhouseconfig.Config) { c.Compression = "bogus" },
			wantErr: true,
		},
		{
			name: "dsn replaces hosts",
			mutate: func(c *clickhouseconfig.Config) {
				c.Hosts = nil
				c.Database = ""
				c.Username = ""
				c.ConnectionURI = redacted.RedactedString("clickhouse://host:9000/db")
			},
		},
		{
			name: "dsn conflicts with username",
			mutate: func(c *clickhouseconfig.Config) {
				c.ConnectionURI = redacted.RedactedString("clickhouse://host:9000/db")
				c.Username = "writer"
			},
			wantErr: true,
		},
		{
			name: "dsn conflicts with password",
			mutate: func(c *clickhouseconfig.Config) {
				c.Username = ""
				c.Password = redacted.RedactedString("p")
				c.ConnectionURI = redacted.RedactedString("clickhouse://host:9000/db")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := clickhouseconfig.Default()
			tt.mutate(&cfg)

			err := cfg.Validate()

			if tt.wantErr {
				require.Error(t, err)

				return
			}
			require.NoError(t, err)
		})
	}
}

// A malformed DSN must not leak its credentials through the error.
func TestClientOptions_InvalidURIHidesCredentials(t *testing.T) {
	t.Parallel()

	cfg := clickhouseconfig.Default()
	cfg.ConnectionURI = "clickhouse://writer:s3cr3t-pass@host:notaport/db"

	_, err := chfactory.New(&cfg).ClientOptions()
	require.ErrorIs(t, err, chfactory.ErrInvalidConnectionURI)
	require.NotContains(t, err.Error(), "s3cr3t-pass")
}

// A `tls` block in the configuration enables TLS without UseTlsConfig; an
// injected configuration takes precedence.
func TestClientOptions_TLSFromConfig(t *testing.T) {
	t.Parallel()

	cfg := clickhouseconfig.Default()
	cfg.TLS = &tlsconfig.Client{ServerName: "clickhouse.internal"}

	opts, err := chfactory.New(&cfg).ClientOptions()
	require.NoError(t, err)
	require.NotNil(t, opts.TLS, "a configured tls block must not fall back to plaintext")
	require.Equal(t, "clickhouse.internal", opts.TLS.ServerName)

	injected := &tls.Config{ServerName: "injected", MinVersion: tls.VersionTLS12}
	opts, err = chfactory.New(&cfg).UseTlsConfig(injected).ClientOptions()
	require.NoError(t, err)
	require.Same(t, injected, opts.TLS)
}
