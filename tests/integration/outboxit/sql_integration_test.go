// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outboxit_test

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/outbox"
	"github.com/altessa-s/go-atlas/tests/integration/outboxit"

	outboxsql "github.com/altessa-s/go-atlas/data/outbox/store/sqldb"
)

// Payload nil-ness survives the store on every backend: nil stays nil, an empty
// payload stays empty, and bytes come back unchanged.
func TestStore_PayloadsRoundTrip(t *testing.T) {
	t.Parallel()

	forEachBackend(t, func(t *testing.T, f *fixture) {
		ob := f.newOutbox(t)
		saved := f.save(t, ob,
			outbox.Event{Key: "k.nil"},
			outbox.Event{Key: "k.empty", Payload: []byte{}},
			outbox.Event{Key: "k.bytes", Payload: []byte{0, 1, 0xff}},
		)

		events, err := f.store.FetchUnprocessedEvents(t.Context(), 10)
		require.NoError(t, err)
		got := map[string][]byte{}
		for _, ev := range events {
			got[ev.Id] = ev.Payload
		}
		require.Nil(t, got[saved[0].Id])
		require.Empty(t, got[saved[1].Id])
		require.Equal(t, []byte{0, 1, 0xff}, got[saved[2].Id])
	})
}

// Unbounded values fit: a key and a handler error larger than 64 KB (a MySQL
// BLOB/TEXT limit) are stored, and a huge error on one event does not roll back
// the result of a successful event updated in the same batch.
func TestStore_LargeKeysAndErrors(t *testing.T) {
	t.Parallel()

	forEachBackend(t, func(t *testing.T, f *fixture) {
		bigKey := "k." + strings.Repeat("k", 70*1024)
		bigErr := errors.New(strings.Repeat("e", 70*1024))
		f.recorder.Respond(func(d outboxit.Delivery) error {
			if d.Key == bigKey {
				return bigErr
			}
			return nil
		})
		ob := f.newOutbox(t)
		saved := f.save(t, ob, event(bigKey, "fails"), event("k.ok", "succeeds"))

		require.NoError(t, ob.RunDispatchCycle(t.Context()))

		failed := f.load(t, saved[0].Id)
		require.Equal(t, string(outbox.StatusFailed), failed.Status)
		require.Equal(t, uint32(1), failed.Attempts)
		require.Equal(t, bigKey, failed.Topic)
		require.NotNil(t, failed.LastError)
		require.Equal(t, bigErr.Error(), *failed.LastError)
		require.Equal(t, string(outbox.StatusSent), f.load(t, saved[1].Id).Status)
	})
}

// A handler error is an arbitrary string — here one containing NUL, which
// PostgreSQL TEXT rejects. It must persist like any other, without rolling back
// the result of a sibling event updated in the same batch, and the failing
// event must still spend its attempts toward the retry limit.
func TestStore_ErrorsWithNULPersist(t *testing.T) {
	t.Parallel()

	forEachBackend(t, func(t *testing.T, f *fixture) {
		nulErr := errors.New("bad\x00payload")
		f.recorder.Respond(func(d outboxit.Delivery) error {
			if d.Key == "k.nul" {
				return nulErr
			}
			return nil
		})
		ob := f.newOutbox(t, outbox.WithRetryMaxAttempts(1))
		saved := f.save(t, ob, event("k.nul", "fails"), event("k.ok", "succeeds"))

		require.NoError(t, ob.RunDispatchCycle(t.Context()))

		failed := f.load(t, saved[0].Id)
		require.Equal(t, string(outbox.StatusMaxAttemptReached), failed.Status)
		require.Equal(t, uint32(1), failed.Attempts)
		require.NotNil(t, failed.LastError)
		require.Equal(t, nulErr.Error(), *failed.LastError)
		require.Equal(t, string(outbox.StatusSent), f.load(t, saved[1].Id).Status)
	})
}

// Client-supplied instants survive the round trip exactly (to the millisecond,
// MongoDB's BSON date precision) regardless of the driver's time zone settings
// — the MySQL backend runs with loc=Asia/Tokyo — and the backlog age is
// measured on the database clock.
func TestStore_InstantsAreNotShifted(t *testing.T) {
	t.Parallel()

	forEachBackend(t, func(t *testing.T, f *fixture) {
		created := time.Now().UTC().Add(-time.Minute).Truncate(time.Millisecond)
		ob := f.newOutbox(t)
		saved := f.save(t, ob, outbox.Event{Key: "k.time", Payload: []byte("x"), CreatedAt: created, ExpiresAt: created.Add(time.Hour)})

		require.Equal(t, created, f.load(t, saved[0].Id).CreatedAt)

		events, err := f.store.FetchUnprocessedEvents(t.Context(), 10)
		require.NoError(t, err)
		require.Len(t, events, 1, "an event expiring in an hour must still be due")
		require.Equal(t, created, events[0].CreatedAt)
		require.Equal(t, created.Add(time.Hour), events[0].ExpiresAt)
		require.NoError(t, f.store.UnlockStuckEvents(t.Context(), 0))

		stats, err := f.store.Stats(t.Context())
		require.NoError(t, err)
		require.InDelta(t, time.Minute.Seconds(), stats.OldestPendingAge.Seconds(), 30, "age is a minute, not hours off")
	})
}

// database/sql has no notification API, so a SQL store reports Watch as
// unsupported and dispatch stays on the poll schedule.
func TestStore_SQLWatchIsUnsupported(t *testing.T) {
	t.Parallel()

	for _, spec := range sqlSpecs() {
		t.Run(spec.name, func(t *testing.T) {
			t.Parallel()
			f := newSQLFixture(t, spec)
			require.ErrorIs(t, f.newOutbox(t).Watch(t.Context()), outbox.ErrWatchUnsupported)
		})
	}
}

// Instances starting together can each create the same absent events table,
// whether they name it schema-qualified or not: EnsureSchema serializes the
// initial DDL rather than racing on the catalog.
func TestStore_ConcurrentEnsureSchema(t *testing.T) {
	t.Parallel()

	for _, spec := range sqlSpecs() {
		t.Run(spec.name, func(t *testing.T) {
			t.Parallel()
			for range 5 {
				f := newSQLFixtureWithoutSchema(t, spec)
				query := "SELECT DATABASE()"
				if spec.dialect == outboxsql.DialectPostgres {
					query = "SELECT current_schema()"
				}
				var schema string
				require.NoError(t, f.db.QueryRowContext(t.Context(), query).Scan(&schema))
				var wg sync.WaitGroup
				errs := make([]error, 8)
				for i := range errs {
					wg.Go(func() {
						// Half the instances name the table schema-qualified.
						table := f.events
						if i%2 == 1 {
							table = schema + "." + table
						}
						store, err := outboxsql.New(f.db, spec.dialect, outboxsql.WithTableName(table))
						if err == nil {
							err = store.EnsureSchema(t.Context())
						}
						errs[i] = err
					})
				}
				wg.Wait()
				for _, err := range errs {
					require.NoError(t, err)
				}
			}
		})
	}
}
