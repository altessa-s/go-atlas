// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo"

	migrate "github.com/xakep666/mongo-migrate"
)

// isolateMigrationRegistry empties the process-wide migration registry for the test and restores it afterwards. Tests using it must
// not run in parallel: the registry is global, and the restore must not race with another test's Connect.
func isolateMigrationRegistry(tb testing.TB) {
	tb.Helper()
	migrationRegistry.mu.Lock()
	saved := migrationRegistry.migrations
	migrationRegistry.migrations = nil
	migrationRegistry.mu.Unlock()
	tb.Cleanup(func() {
		migrationRegistry.mu.Lock()
		migrationRegistry.migrations = saved
		migrationRegistry.mu.Unlock()
	})
}

func noopMigration(context.Context, *mongo.Database) error { return nil }

// TestRegisterMigration is serial: it swaps the process-wide migration registry.
func TestRegisterMigration(t *testing.T) { //nolint:paralleltest // swaps the process-wide migration registry
	isolateMigrationRegistry(t)

	require.ErrorIs(t, RegisterMigration(0, "zero", noopMigration, nil), ErrInvalidMigration)
	require.ErrorIs(t, RegisterMigration(3, "no up", nil, nil), ErrInvalidMigration)

	require.NoError(t, RegisterMigration(3, "third", noopMigration, nil))
	require.NoError(t, RegisterMigration(1, "first", noopMigration, noopMigration))
	require.ErrorIs(t, RegisterMigration(3, "again", noopMigration, nil), ErrInvalidMigration)
	require.Panics(t, func() { MustRegisterMigration(1, "again", noopMigration, nil) })

	got := RegisteredMigrations()
	require.Len(t, got, 2)
	require.Equal(t, uint64(1), got[0].Version)
	require.Equal(t, "first", got[0].Description)
	require.Equal(t, uint64(3), got[1].Version)
	require.Equal(t, "third", got[1].Description)
	require.NotNil(t, got[1].Down, "a nil down becomes a no-op")
	require.NoError(t, got[1].Down(t.Context(), nil))

	got[0].Description = "mutated"
	require.Equal(t, "first", RegisteredMigrations()[0].Description, "callers get a copy")
}

func TestMergeMigrations(t *testing.T) {
	t.Parallel()

	global := []migrate.Migration{{Version: 1, Description: "global", Up: noopMigration, Down: noopMigration}}
	typed := []Migration{{Version: 2, Description: "typed", Up: noopMigration, Down: noopMigration}}

	merged, err := mergeMigrations(global, typed)
	require.NoError(t, err)
	require.Len(t, merged, 2)
	require.Equal(t, uint64(1), merged[0].Version)
	require.Equal(t, uint64(2), merged[1].Version)
	require.Equal(t, "typed", merged[1].Description)
	require.NotNil(t, merged[1].Up)
	require.NotNil(t, merged[1].Down)

	_, err = mergeMigrations(global, []Migration{{Version: 1, Up: noopMigration, Down: noopMigration}})
	require.ErrorIs(t, err, ErrInvalidMigration, "one version in both registries")

	merged, err = mergeMigrations(nil, nil)
	require.NoError(t, err)
	require.Empty(t, merged)
}
