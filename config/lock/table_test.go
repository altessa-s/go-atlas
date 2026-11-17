// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package lockconfig_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	lockconfig "github.com/altessa-s/go-atlas/config/lock"
)

func TestSQLTableNameIsValidated(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		table string
		valid bool
	}{
		{"items", true},
		{"app.items", true},
		{"bad-table", false},
		{"a.b.c", false},
	} {
		t.Run(tc.table, func(t *testing.T) {
			t.Parallel()
			c := lockconfig.DistributionLockSQL{Dialect: "postgres", Table: tc.table, PurgeBatchSize: 1}
			if tc.valid {
				require.NoError(t, c.Validate())
			} else {
				require.Error(t, c.Validate())
			}
		})
	}
}
