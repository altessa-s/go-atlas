package mongo

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestExpiredCleanupOptionMigrationBehavior(t *testing.T) {
	for _, grace := range []time.Duration{0, -time.Second, time.Second} {
		o := &options{}
		WithExpiredCleanup(grace)(o)
		require.True(t, o.expiredCleanup)
		require.Equal(t, grace, o.expiredCleanupGrace)
	}
}
