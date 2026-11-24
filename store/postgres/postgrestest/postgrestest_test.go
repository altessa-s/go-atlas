// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package postgrestest_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/store/postgres/postgrestest"
)

// Serial because the gate is a process environment variable.
func TestNewSkipsWithoutDSN(t *testing.T) {
	t.Setenv("POSTGRESTEST_EMPTY_DSN", "")

	var skipped bool
	t.Run("gated", func(t *testing.T) {
		defer func() { skipped = t.Skipped() }()
		postgrestest.New(t, postgrestest.WithEnv("POSTGRESTEST_EMPTY_DSN"))
		t.Error("New returned without a DSN")
	})
	require.True(t, skipped)
}
