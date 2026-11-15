// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package lockconfig_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	lockconfig "github.com/altessa-s/go-atlas/config/lock"
	storageconfig "github.com/altessa-s/go-atlas/config/storage"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

func lockFieldErrors(t *testing.T, err error) validation.Errors {
	t.Helper()
	verrs, ok := errors.AsType[validation.Errors](err)
	require.True(t, ok, "want validation.Errors, got %v", err)
	return verrs
}

func TestDistributionLockNATS_Validate(t *testing.T) {
	t.Parallel()

	n := lockconfig.DefaultDistributionLockNATS()
	require.Equal(t, "dlock", n.Bucket)
	require.Equal(t, storageconfig.KVStorageMemory, n.Storage)
	require.NoError(t, n.Validate())

	n.Storage = storageconfig.KVStorageFile
	require.NoError(t, n.Validate())

	n.Storage = "disk"
	require.Contains(t, lockFieldErrors(t, n.Validate()), "Storage")
}

func TestDistributionLockSQL_Validate(t *testing.T) {
	t.Parallel()

	require.NoError(t, (&lockconfig.DistributionLockSQL{Dialect: lockconfig.SQLDialectMySQL, Table: "locks"}).Validate())

	verrs := lockFieldErrors(t, (&lockconfig.DistributionLockSQL{}).Validate())
	require.Contains(t, verrs, "Dialect")
	require.Contains(t, verrs, "Table")

	require.Contains(t, lockFieldErrors(t, (&lockconfig.DistributionLockSQL{Dialect: "sqlite", Table: "t"}).Validate()), "Dialect")
}

func TestDistributionLock_Validate(t *testing.T) {
	t.Parallel()

	empty := lockconfig.DefaultDistributionLock()
	require.Contains(t, lockFieldErrors(t, empty.Validate()), "Provider")
	require.Contains(t, lockFieldErrors(t, (&lockconfig.DistributionLock{Provider: "etcd"}).Validate()), "Provider")

	require.NoError(t, (&lockconfig.DistributionLock{Provider: lockconfig.DistributionLockProviderNATS}).Validate(),
		"nats sub-config is optional")
	natsCfg := lockconfig.DefaultDistributionLockNATS()
	require.NoError(t, (&lockconfig.DistributionLock{Provider: lockconfig.DistributionLockProviderNATS, NATS: &natsCfg}).Validate())

	require.Contains(t, lockFieldErrors(t, (&lockconfig.DistributionLock{
		Provider: lockconfig.DistributionLockProviderMongo,
	}).Validate()), "Mongo")
	require.Contains(t, lockFieldErrors(t, (&lockconfig.DistributionLock{
		Provider: lockconfig.DistributionLockProviderSQL,
	}).Validate()), "SQL")

	mongoCfg := lockconfig.DefaultDistributionLockMongo()
	require.NoError(t, (&lockconfig.DistributionLock{
		Provider: lockconfig.DistributionLockProviderMongo, Mongo: &mongoCfg,
	}).Validate())
	require.NoError(t, (&lockconfig.DistributionLock{
		Provider: lockconfig.DistributionLockProviderSQL,
		SQL:      &lockconfig.DistributionLockSQL{Dialect: lockconfig.SQLDialectPostgres, Table: "dlocks"},
	}).Validate())
}

func TestLeaderElector_Validate(t *testing.T) {
	t.Parallel()

	le := lockconfig.DefaultLeaderElector()
	require.Equal(t, 10*time.Second, le.Ttl)
	require.Equal(t, lockconfig.LeaderElectorProviderNATS, le.Provider, "matches the provider default tag")
	require.NoError(t, le.Validate())

	tests := []struct {
		name  string
		mut   func(*lockconfig.LeaderElector)
		field string
	}{
		{"unknown provider", func(c *lockconfig.LeaderElector) { c.Provider = "zookeeper" }, "Provider"},
		{"ttl below one second", func(c *lockconfig.LeaderElector) { c.Ttl = 500 * time.Millisecond }, "Ttl"},
		{"unknown storage", func(c *lockconfig.LeaderElector) { c.Storage = "disk" }, "Storage"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := le
			tc.mut(&c)
			require.Contains(t, lockFieldErrors(t, c.Validate()), tc.field)
		})
	}
}
