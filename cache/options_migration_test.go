package cache

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOptionMigrationBehavior(t *testing.T) {
	t.Run("isolated results", func(t *testing.T) {
		o := &options{}
		WithIsolatedResults()(o)
		require.True(t, o.isolatedResults)
	})
	t.Run("key version replaces generation", func(t *testing.T) {
		version := KeyVersionFunc(func(context.Context) (uint64, error) { return 1, nil })
		o := &options{invalidate: func(context.Context) error { return nil }}
		WithKeyVersion(version)(o)
		got, err := o.keyVersion(t.Context())
		require.NoError(t, err)
		require.Equal(t, uint64(1), got)
		require.Nil(t, o.invalidate)
		WithKeyVersion(nil)(o)
		require.Nil(t, o.keyVersion)
		require.Nil(t, o.invalidate)
	})
}
