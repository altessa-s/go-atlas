// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package migrations

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"
)

func noopMigration(context.Context, *sql.Tx) error { return nil }

// TestRegistryConformance is the registry rules table shared, verbatim, with store/mongo/migrations: a nil down is the irreversible
// sentinel, a version below 1 and a version registered twice are rejected, a nil up is rejected.
func TestRegistryConformance(t *testing.T) {
	t.Parallel()

	var r Registry
	require.ErrorIs(t, r.Register(0, "zero", noopMigration, noopMigration), ErrInvalidMigration)
	require.ErrorIs(t, r.Register(1, "no up", nil, noopMigration), ErrInvalidMigration)
	require.NoError(t, r.Register(1, "nil down", noopMigration, nil))
	require.ErrorIs(t, r.Register(1, "duplicate", noopMigration, noopMigration), ErrInvalidMigration)

	got := r.Registered()
	require.Len(t, got, 1)
	require.ErrorIs(t, got[0].Down(t.Context(), nil), ErrIrreversibleMigration)
}
