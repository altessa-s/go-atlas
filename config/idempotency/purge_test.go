// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotencyconfig_test

import (
	"testing"
	"time"

	idempotencyconfig "github.com/altessa-s/go-atlas/config/idempotency"
	"github.com/stretchr/testify/require"
)

func TestSQLPurgeSettings(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		interval time.Duration
		batch    int
		valid    bool
	}{
		{"defaults", time.Minute, 1000, true},
		{"manual", 0, 3, true},
		{"negative interval", -time.Second, 1000, false},
		{"negative batch", time.Minute, -1, false},
		{"largest MySQL key batch", time.Minute, 65_534, true},
		{"batch over the MySQL placeholder limit", time.Minute, 65_535, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := idempotencyconfig.SQLStorageConfig{Dialect: "postgres", Table: "items", PurgeInterval: tc.interval, PurgeBatchSize: tc.batch}
			if tc.valid {
				require.NoError(t, c.Validate())
			} else {
				require.Error(t, c.Validate())
			}
		})
	}
}
