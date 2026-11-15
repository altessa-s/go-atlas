package outbox

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOptionMigrationBehavior(t *testing.T) {
	t.Run("callbacks preserve nil and assignment", func(t *testing.T) {
		o := &options{shouldRetry: func(error) bool { return true }}
		WithShouldRetry(nil)(o)
		require.Nil(t, o.shouldRetry)
		fn := func(error) bool { return false }
		WithShouldRetry(fn)(o)
		require.False(t, o.shouldRetry(errors.New("x")))
	})
	t.Run("compaction side effects", func(t *testing.T) {
		o := &options{compactionFilter: func(string) bool { return true }}
		WithCompaction()(o)
		require.True(t, o.compaction)
		require.Nil(t, o.compactionFilter)
		filter := func(s string) bool { return s == "x" }
		WithCompactionFilter(nil)(o)
		require.True(t, o.compaction)
		require.Nil(t, o.compactionFilter)
		WithCompactionFilter(filter)(o)
		require.True(t, o.compaction)
		require.True(t, o.compactionFilter("x"))
	})
	t.Run("guards keep invalid inputs", func(t *testing.T) {
		o := &options{publishedEventsLifetime: time.Hour, defaultEventTTL: time.Hour, maxPayloadBytes: 12}
		WithPublishedEventsLifetime(0)(o)
		WithDefaultEventTTL(time.Millisecond)(o)
		WithMaxPayloadBytes(-1)(o)
		require.Equal(t, time.Hour, o.publishedEventsLifetime)
		require.Equal(t, time.Hour, o.defaultEventTTL)
		require.Equal(t, 12, o.maxPayloadBytes)
		WithPublishedEventsLifetime(time.Minute)(o)
		WithDefaultEventTTL(0)(o)
		WithMaxPayloadBytes(0)(o)
		require.Equal(t, time.Minute, o.publishedEventsLifetime)
		require.Equal(t, time.Hour, o.defaultEventTTL)
		require.Zero(t, o.maxPayloadBytes)
	})
}
