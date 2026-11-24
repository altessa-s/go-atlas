// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package postgresconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-nomos/loader"
)

func TestConfig(t *testing.T) {
	c := Default()
	require.Equal(t, 30*time.Second, c.MigrationTimeout)
	require.Error(t, c.Validate())
	c.DSN = "postgres://name:hidden@localhost/db"
	require.NoError(t, c.Validate())
	require.NotContains(t, fmt.Sprint(c), "hidden")
	c.MaxOpenConnections = 2
	c.Default()
	require.Equal(t, 2, c.MaxIdleConnections)
	c.Pools = map[string]Pool{"reader": {MaxOpenConnections: 1}}
	require.NoError(t, c.Validate())
	for _, mutate := range []func(*Config){
		func(c *Config) { c.MigrationTimeout = 0 },
		func(c *Config) { c.MigrationTimeout = time.Millisecond - 1 },
		func(c *Config) { c.DSN = "opaque" }, func(c *Config) { c.Pooling = "session" }, func(c *Config) { c.MaxIdleConnections = 3 },
		func(c *Config) { c.LockTimeout = time.Nanosecond }, func(c *Config) { c.CountBudget = -1 }, func(c *Config) { c.Pools = map[string]Pool{"foreground": {}} },
		func(c *Config) { c.Pools = map[string]Pool{"worker": {MaxOpenConnections: 1, LockTimeout: -1}} },
		func(c *Config) {
			n := 2
			c.Pools = map[string]Pool{"worker": {MaxOpenConnections: 1, MaxIdleConnections: &n}}
		},
	} {
		bad := c
		mutate(&bad)
		require.Error(t, bad.Validate())
	}
	c.MigrationTimeout = time.Millisecond
	c.CountBudget = 0
	c.DSN = "host=localhost dbname=atlas"
	require.NoError(t, c.Validate())
}

func TestLoadPoolDefaults(t *testing.T) {
	for _, tc := range []struct {
		yaml   string
		idle   int
		budget time.Duration
	}{{"", 3, 30 * time.Second}, {"  maxIdleConnections: 0\n  migrationTimeout: 2m\n", 0, 2 * time.Minute}} {
		path := filepath.Join(t.TempDir(), "config.yaml")
		data := "postgres:\n  dsn: host=localhost\n  maxOpenConnections: 3\n  countBudget: 0s\n" + tc.yaml +
			"  pools:\n    jobs:\n      maxOpenConnections: 2\n      maxIdleConnections: 0\n"
		require.NoError(t, os.WriteFile(path, []byte(data), 0o600))
		var c struct {
			Postgres *Config `yaml:"postgres"`
		}
		_, err := loader.New(nil, loader.WithPath(path), loader.WithSkipEnv()).Load(&c)
		require.NoError(t, err)
		require.NoError(t, c.Postgres.Validate())
		require.Equal(t, tc.idle, c.Postgres.MaxIdleConnections)
		require.Equal(t, tc.budget, c.Postgres.MigrationTimeout)
		require.Zero(t, c.Postgres.CountBudget)
		require.NotNil(t, c.Postgres.Pools["jobs"].MaxIdleConnections)
		require.Zero(t, *c.Postgres.Pools["jobs"].MaxIdleConnections)
	}
}
