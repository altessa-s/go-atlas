// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auditit_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-core/time/clock"
	"github.com/altessa-s/go-atlas/audit"
	"github.com/altessa-s/go-atlas/audit/storages/sqldb"
)

func TestSQLRetention(t *testing.T) {
	t.Parallel()
	for _, target := range sqlTargets() {
		t.Run(target.name, func(t *testing.T) {
			t.Parallel()
			at := time.Now().Truncate(time.Millisecond)
			st := target.storage(t, sqldb.WithTTL(time.Hour), sqldb.WithClock(clock.NewFake(at)),
				sqldb.WithPurgeInterval(0), sqldb.WithPurgeBatchSize(2))
			cutoff := at.Add(-time.Hour)
			require.NoError(t, st.StoreBatch(t.Context(), []*audit.Event{
				event("old1", cutoff.Add(-time.Hour)), event("old2", cutoff.Add(-time.Second)),
				event("old3", cutoff.Add(-time.Millisecond)), event("boundary", cutoff), event("fresh", at),
			}))
			n, err := st.PurgeExpired(t.Context())
			require.NoError(t, err)
			require.EqualValues(t, 3, n)
			require.Equal(t, []string{"boundary", "fresh"}, ids(t, st))
			n, err = st.PurgeExpired(t.Context())
			require.NoError(t, err)
			require.Zero(t, n)
		})
	}
}

func TestSQLRetentionLoop(t *testing.T) {
	t.Parallel()
	for _, target := range sqlTargets() {
		t.Run(target.name, func(t *testing.T) {
			t.Parallel()
			at := time.Now().Truncate(time.Millisecond)
			st := target.storage(t, sqldb.WithTTL(time.Hour), sqldb.WithClock(clock.NewFake(at)),
				sqldb.WithPurgeInterval(20*time.Millisecond), sqldb.WithPurgeBatchSize(1))
			require.NoError(t, st.StoreBatch(t.Context(), []*audit.Event{
				event("old1", at.Add(-2*time.Hour)), event("old2", at.Add(-3*time.Hour)), event("fresh", at),
			}))
			require.Eventually(t, func() bool {
				n, err := st.Count(t.Context(), &audit.Query{})
				return err == nil && n == 1
			}, 3*time.Second, 10*time.Millisecond)
			require.Equal(t, []string{"fresh"}, ids(t, st))
			require.NoError(t, st.Close(t.Context()))
			require.NoError(t, st.Store(t.Context(), event("old-after-close", at.Add(-2*time.Hour))))
			require.Never(t, func() bool {
				n, err := st.Count(t.Context(), &audit.Query{})
				return err != nil || n != 2
			}, 100*time.Millisecond, 10*time.Millisecond, "Close stops purging and keeps the database open")
		})
	}
}
