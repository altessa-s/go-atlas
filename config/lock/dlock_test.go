// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package lockconfig_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	lockconfig "github.com/altessa-s/go-atlas/config/lock"
)

func TestDistributionLockMongo_Validate(t *testing.T) {
	t.Parallel()

	valid := lockconfig.DefaultDistributionLockMongo()
	require.Equal(t, 10*time.Second, valid.TTL)
	require.NoError(t, valid.Validate())

	tests := map[string]struct {
		mutate func(*lockconfig.DistributionLockMongo)
		ok     bool
	}{
		"zero ttl keeps the provider default": {mutate: func(c *lockconfig.DistributionLockMongo) { c.TTL = 0 }, ok: true},
		"ttl below 1ms":                       {mutate: func(c *lockconfig.DistributionLockMongo) { c.TTL = time.Microsecond }},
		"renew ratio inside (0, 1)":           {mutate: func(c *lockconfig.DistributionLockMongo) { c.RenewRatio = 0.5 }, ok: true},
		"renew ratio 1":                       {mutate: func(c *lockconfig.DistributionLockMongo) { c.RenewRatio = 1 }},
		"negative renew ratio":                {mutate: func(c *lockconfig.DistributionLockMongo) { c.RenewRatio = -0.1 }},
		"hard lease with cleanup": {mutate: func(c *lockconfig.DistributionLockMongo) {
			c.WithoutRenewal, c.ExpiredCleanup, c.ExpiredCleanupGrace, c.EnsureIndexes = true, true, time.Hour, true
		}, ok: true},
		"negative grace": {mutate: func(c *lockconfig.DistributionLockMongo) { c.ExpiredCleanupGrace = -time.Second }},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c := lockconfig.DefaultDistributionLockMongo()
			tc.mutate(&c)
			if tc.ok {
				require.NoError(t, c.Validate())
				return
			}
			require.Error(t, c.Validate())
			dl := lockconfig.DistributionLock{Provider: lockconfig.DistributionLockProviderMongo, Mongo: &c}
			require.Error(t, dl.Validate(), "the parent validates the mongo section")
		})
	}
}
