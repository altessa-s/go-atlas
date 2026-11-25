// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package retryqueueconfig_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	retryqueueconfig "github.com/altessa-s/go-atlas/config/retryqueue"
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
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := retryqueueconfig.SQLStorageConfig{Dialect: "postgres", Table: "items", PurgeInterval: tc.interval, PurgeBatchSize: tc.batch}
			if tc.valid {
				require.NoError(t, c.Validate())
			} else {
				require.Error(t, c.Validate())
			}
		})
	}
}
