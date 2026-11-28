// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package postgresconfig

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-nomos/loader"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	tlsconfig "github.com/altessa-s/go-atlas/config/tls"
)

func TestMigrationsValidate(t *testing.T) {
	t.Parallel()
	primary := Default()
	primary.DSN = "host=pooler"
	m := DefaultMigrations()
	require.Equal(t, 5*time.Second, m.LockTimeout)
	require.NoError(t, m.Validate(primary), "the DSN is optional")
	m.DSN = "postgres://owner:hidden@db/app"
	require.NoError(t, m.Validate(primary))
	m.LockTimeout = primary.MigrationTimeout
	require.NoError(t, m.Validate(primary), "the whole migration budget")

	for name, mutate := range map[string]func(*Migrations, *Config){
		"zero":        func(m *Migrations, _ *Config) { m.LockTimeout = 0 },
		"sub-ms":      func(m *Migrations, _ *Config) { m.LockTimeout = time.Microsecond },
		"over budget": func(m *Migrations, c *Config) { m.LockTimeout = c.MigrationTimeout + time.Millisecond },
		"server range": func(m *Migrations, c *Config) {
			c.MigrationTimeout = time.Hour * 1000
			m.LockTimeout = math.MaxInt32*time.Millisecond + time.Millisecond
		},
		"opaque DSN":    func(m *Migrations, _ *Config) { m.DSN = "opaque" },
		"malformed URL": func(m *Migrations, _ *Config) { m.DSN = "postgres://owner:hidden@db:bad/app" },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			bad, c := m, primary
			bad.LockTimeout = time.Second
			mutate(&bad, &c)
			err := bad.Validate(c)
			require.Error(t, err)
			require.NotContains(t, err.Error(), "hidden")
		})
	}
	err := Migrations{LockTimeout: time.Minute}.Validate(primary)
	require.Contains(t, strings.ToLower(err.Error()), "locktimeout")

	tlsPrimary := primary
	tlsPrimary.TLS = &tlsconfig.Client{}
	require.ErrorIs(t, Migrations{DSN: "host=db sslmode=disable", LockTimeout: time.Second}.Validate(tlsPrimary), coreerrs.ErrInvalidArgument,
		"the session inherits the primary TLS block")
	require.NoError(t, Migrations{DSN: "host=db sslmode=verify-full", LockTimeout: time.Second}.Validate(tlsPrimary))
}

func TestMigrationSession(t *testing.T) {
	t.Parallel()
	primary := Default()
	primary.DSN = "host=pooler"
	primary.Pooling = transaction
	primary.TLS = &tlsconfig.Client{ServerName: "db.internal"}
	primary.Pools = map[string]Pool{"worker": {MaxOpenConnections: 4}}
	primary.MigrationTimeout = 2 * time.Minute
	m := Migrations{DSN: "host=db", LockTimeout: time.Second}

	got := primary.MigrationSession(m)
	require.NoError(t, got.Validate())
	require.Equal(t, m.DSN, got.DSN)
	require.Equal(t, direct, got.Pooling)
	require.Nil(t, got.Pools)
	require.Equal(t, 1, got.MaxOpenConnections)
	require.Equal(t, 1, got.MaxIdleConnections)
	require.Equal(t, 2*time.Minute, got.StatementTimeout)
	require.Equal(t, 2*time.Minute, got.LockTimeout)
	require.Same(t, primary.TLS, got.TLS)
	require.Equal(t, primary.ConnectTimeout, got.ConnectTimeout)
	require.Equal(t, primary.MigrationTimeout, got.MigrationTimeout)
	require.Equal(t, "host=pooler", primary.DSN.Expose(), "the primary configuration is not modified")
	require.Len(t, primary.Pools, 1)
	require.NotContains(t, fmt.Sprint(got), "host=db")

	primary.MigrationTimeout = 1000 * time.Hour
	capped := primary.MigrationSession(m)
	require.Equal(t, math.MaxInt32*time.Millisecond, capped.StatementTimeout, "capped at the server range")
	require.NoError(t, capped.Validate())
}

func TestLoadMigrations(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		yaml string
		want time.Duration
	}{{"migrations:\n  dsn: host=db\n", 5 * time.Second}, {"migrations:\n  lockTimeout: 250ms\n", 250 * time.Millisecond}, {"", 5 * time.Second}} {
		path := filepath.Join(t.TempDir(), "config.yaml")
		require.NoError(t, os.WriteFile(path, []byte("postgres:\n  dsn: host=localhost\n"+tc.yaml), 0o600))
		var c struct {
			Postgres   *Config    `yaml:"postgres"`
			Migrations Migrations `yaml:"migrations"`
		}
		_, err := loader.New(nil, loader.WithPath(path), loader.WithSkipEnv()).Load(&c)
		require.NoError(t, err)
		require.Equal(t, tc.want, c.Migrations.LockTimeout)
		require.NoError(t, c.Migrations.Validate(*c.Postgres))
	}
}
