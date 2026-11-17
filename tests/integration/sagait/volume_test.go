//go:build volume

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sagait_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas-core/runtime/concurrency"
	"github.com/altessa-s/go-atlas/internal/sqldialect"
	"github.com/altessa-s/go-atlas/reliability/saga"
	"github.com/altessa-s/go-atlas/reliability/saga/storages/sqldb"
	"github.com/altessa-s/go-atlas/tests/integration/internal/livebackend"

	mongostore "github.com/altessa-s/go-atlas/reliability/saga/storages/mongo"
)

// Volume data set: most instances are terminal, as in a long-running
// deployment; the rest mix every recovery case, so the expected recoverable
// set is computed with saga.Instance.Recoverable itself.
const (
	volDefA = "def-a"
	volDefB = "def-b"
	// volRecoverLimit is a recovery batch, as WithRecoveryBatchSize sets it.
	volRecoverLimit = 100
	// volRaceInstances instances are each updated by volRaceWriters writers
	// that all hold the same version.
	volRaceInstances = 200
	volRaceWriters   = 8
)

// volNow is the recovery time; whole seconds, as the MongoDB store keeps
// deadlines in seconds.
var volNow = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// volumeInstance returns instance i of the data set, by i%100:
//
//	0-59 COMPLETED, 60-74 COMPENSATED, 75-94 FAILED (even ones with an expired
//	     lease and a passed deadline: terminal wins);
//	  95 RUNNING with an expired lease (recoverable);
//	  96 RUNNING with an active lease and a passed deadline (not: the lease wins);
//	  97 RUNNING without owner: a passed, a future or no deadline by (i/100)%3;
//	  98 COMPENSATING without lease or with an expired one (recoverable);
//	  99 COMPENSATING with an active lease (not).
func volumeInstance(i int) *saga.Instance {
	inst := &saga.Instance{
		ID: fmt.Sprintf("saga-%09d", i), Definition: volDefA, Stage: 1,
		CreatedAt: volNow.Add(-2 * time.Hour), UpdatedAt: volNow.Add(-time.Hour), Version: int64(i % 5),
	}
	if i%2 == 1 {
		inst.Definition = volDefB
	}
	past, future, passed := volNow.Add(-time.Minute), volNow.Add(time.Minute), volNow.Add(-time.Hour)
	switch r := i % 100; {
	case r < 60:
		inst.Status = saga.StatusCompleted
	case r < 75:
		inst.Status = saga.StatusCompensated
	case r < 95:
		inst.Status = saga.StatusFailed
		if r%2 == 0 {
			inst.LeaseOwner, inst.LeaseUntil, inst.Deadline = "w0", past, passed
		}
	case r == 95:
		inst.Status, inst.LeaseOwner, inst.LeaseUntil = saga.StatusRunning, "w1", past
	case r == 96:
		inst.Status, inst.LeaseOwner, inst.LeaseUntil, inst.Deadline = saga.StatusRunning, "w2", future, passed
	case r == 97:
		inst.Status = saga.StatusRunning
		switch (i / 100) % 3 {
		case 0:
			inst.Deadline = passed
		case 1:
			inst.Deadline = future
		}
	case r == 98:
		inst.Status = saga.StatusCompensating
		if (i/100)%2 == 0 {
			inst.LeaseUntil = past
		}
	default:
		inst.Status, inst.LeaseOwner, inst.LeaseUntil = saga.StatusCompensating, "w3", future
	}
	return inst
}

// expectedRecoverable returns the IDs of the recoverable instances of the
// data set, restricted to definition when it is non-empty.
func expectedRecoverable(n int, definition string) map[string]struct{} {
	out := make(map[string]struct{})
	for i := range n {
		if inst := volumeInstance(i); inst.Recoverable(volNow) && (definition == "" || inst.Definition == definition) {
			out[inst.ID] = struct{}{}
		}
	}
	return out
}

// checkRecoverable runs FetchRecoverable without a limit, per definition and
// with a recovery batch limit, and compares each result with the expected set
// exactly: no instance missing, none extra, none twice.
func checkRecoverable(t *testing.T, store saga.Storage, n int) {
	t.Helper()
	for _, def := range []string{"", volDefA} {
		want := expectedRecoverable(n, def)
		var got []*saga.Instance
		livebackend.Measure(t, fmt.Sprintf("FetchRecoverable(%q)", def), len(want), func() {
			var err error
			got, err = store.FetchRecoverable(t.Context(), def, volNow, 0)
			require.NoError(t, err)
		})
		seen := make(map[string]struct{}, len(got))
		for _, inst := range got {
			_, dup := seen[inst.ID]
			require.False(t, dup, "%s returned twice", inst.ID)
			seen[inst.ID] = struct{}{}
			require.True(t, inst.Recoverable(volNow), "%s is not recoverable", inst.ID)
		}
		require.Equal(t, want, seen, "definition %q", def)

		limited, err := store.FetchRecoverable(t.Context(), def, volNow, volRecoverLimit)
		require.NoError(t, err)
		require.Len(t, limited, min(volRecoverLimit, len(want)))
		for _, inst := range limited {
			require.Contains(t, want, inst.ID)
		}
	}
}

// checkConcurrentUpdates gives volRaceWriters writers the same version of
// each of volRaceInstances RUNNING instances and lets them all Update at once:
// exactly one writer per instance wins, every other one gets
// ErrVersionConflict, and the stored instance is the winner's.
func checkConcurrentUpdates(t *testing.T, store saga.Storage, n int) {
	t.Helper()
	var snaps []*saga.Instance
	for i := 96; i < n && len(snaps) < volRaceInstances; i += 100 {
		inst, err := store.Get(t.Context(), volumeInstance(i).ID)
		require.NoError(t, err)
		snaps = append(snaps, inst)
	}
	require.NotEmpty(t, snaps)
	wins := make([]atomic.Int32, len(snaps))
	winner := make([]atomic.Int32, len(snaps))
	var conflicts atomic.Int64
	work := make([]int, len(snaps)*volRaceWriters)
	for i := range work {
		work[i] = i
	}
	livebackend.Measure(t, "concurrent Update", len(work), func() {
		require.NoError(t, concurrency.Process(t.Context(), work, func(ctx context.Context, w int) error {
			k := w / volRaceWriters
			inst := *snaps[k]
			inst.LastError = fmt.Sprintf("writer-%d", w)
			switch err := store.Update(ctx, &inst); {
			case err == nil:
				wins[k].Add(1)
				winner[k].Store(int32(w))
			case errors.Is(err, saga.ErrVersionConflict):
				conflicts.Add(1)
			default:
				return err
			}
			return nil
		}, concurrency.WithConcurrency[int](32)))
	})
	require.EqualValues(t, len(snaps)*(volRaceWriters-1), conflicts.Load())
	for k, snap := range snaps {
		require.EqualValues(t, 1, wins[k].Load(), snap.ID)
		got, err := store.Get(t.Context(), snap.ID)
		require.NoError(t, err)
		require.Equal(t, snap.Version+1, got.Version, snap.ID)
		require.Equal(t, fmt.Sprintf("writer-%d", winner[k].Load()), got.LastError, snap.ID)
	}
}

// nanos is the SQL store's time encoding: Unix nanoseconds, 0 for the zero
// time (reliability/saga/storages/sqldb/store.go unixNano).
func nanos(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixNano()
}

// sqlRecoverable is the recovery statement of sqldb.Store.buildStatements
// (reliability/saga/storages/sqldb/store.go), with its arguments.
func sqlRecoverable(table, definition string, limit int) (string, []any) {
	q := "SELECT id, definition, status, stage, pending_steps, data, steps, created_at, updated_at, deadline, " +
		"lease_owner, lease_until, version, last_error FROM " + table +
		" WHERE status IN (?, ?) AND lease_until <= ?" +
		" AND (status = ? OR lease_owner <> '' OR (deadline <> 0 AND deadline <= ?))"
	at := volNow.UnixNano()
	args := []any{string(saga.StatusRunning), string(saga.StatusCompensating), at, string(saga.StatusCompensating), at}
	if definition != "" {
		q, args = q+" AND definition = ?", append(args, definition)
	}
	if limit > 0 {
		q, args = q+" LIMIT ?", append(args, limit)
	}
	return q, args
}

// TestVolume_SagaSQL seeds VolumeRows saga instances across every state in
// the SQL store's row format, checks that FetchRecoverable returns exactly the
// recoverable ones and that its statement is served by a recovery index, and
// races version-checked updates.
func TestVolume_SagaSQL(t *testing.T) {
	n := livebackend.VolumeRows(t)
	for _, target := range sqlTargets() {
		t.Run(target.name, func(t *testing.T) {
			db, table := target.open(t)
			store, err := sqldb.New(db, target.dialect, sqldb.WithTableName(table))
			require.NoError(t, err)
			require.NoError(t, store.EnsureSchema(t.Context()))

			cols := []string{"id", "definition", "status", "stage", "pending_steps", "data", "steps", "created_at", "updated_at",
				"deadline", "lease_owner", "lease_until", "version", "last_error"}
			livebackend.Measure(t, "seed", n, func() {
				livebackend.SQLBulkInsert(t, db, target.Dialect, table, cols, n, func(i int) []any {
					inst := volumeInstance(i)
					return []any{inst.ID, inst.Definition, string(inst.Status), int64(inst.Stage), "", []byte{}, "",
						nanos(inst.CreatedAt), nanos(inst.UpdatedAt), nanos(inst.Deadline), inst.LeaseOwner, nanos(inst.LeaseUntil),
						inst.Version, ""}
				})
			})
			analyzeSQL(t, db, target, table)

			keys := []string{mysqlIndex(table, "lease"), mysqlIndex(table, "deadline")}
			for _, def := range []string{"", volDefA} {
				for _, limit := range []int{0, volRecoverLimit} {
					q, args := sqlRecoverable(table, def, limit)
					if target.dialect == sqldb.DialectPostgres {
						t.Logf("recoverable(%q, %d) plan indexes: %v", def, limit,
							livebackend.ExplainPostgres(t, db, table, sqldialect.Postgres.Bind(q, 1), args...))
						continue
					}
					access, key := livebackend.ExplainMySQL(t, db, table, keys, q, args...)
					t.Logf("recoverable(%q, %d) plan: type=%s key=%s", def, limit, access, key)
				}
			}

			checkRecoverable(t, store, n)
			checkConcurrentUpdates(t, store, n)
		})
	}
}

func mysqlIndex(table, suffix string) string {
	return strings.Trim(sqldialect.MySQL.IndexName(table, suffix), "`")
}

// analyzeSQL refreshes the planner statistics of table after seeding.
func analyzeSQL(t *testing.T, db *sql.DB, target sqlTarget, table string) {
	t.Helper()
	stmt := "ANALYZE TABLE " + table
	if target.dialect == sqldb.DialectPostgres {
		stmt = "ANALYZE " + table
	}
	rows, err := db.QueryContext(t.Context(), stmt)
	require.NoError(t, err)
	require.NoError(t, rows.Close())
}

// mongoInstance is instance i in the MongoDB store's document format
// (reliability/saga/storages/mongo/entity.go): instance times in Unix
// seconds, the lease in nanoseconds, zero values omitted.
func mongoInstance(i int) bson.D {
	inst := volumeInstance(i)
	doc := bson.D{
		{Key: "_id", Value: inst.ID}, {Key: "definition", Value: inst.Definition}, {Key: "status", Value: string(inst.Status)},
		{Key: "stage", Value: inst.Stage}, {Key: "created_at", Value: inst.CreatedAt.Unix()},
		{Key: "updated_at", Value: inst.UpdatedAt.Unix()}, {Key: "version", Value: inst.Version},
	}
	if inst.LeaseOwner != "" {
		doc = append(doc, bson.E{Key: "lease_owner", Value: inst.LeaseOwner})
	}
	if !inst.LeaseUntil.IsZero() {
		doc = append(doc, bson.E{Key: "lease_until", Value: inst.LeaseUntil.UnixNano()})
	}
	if !inst.Deadline.IsZero() {
		doc = append(doc, bson.E{Key: "deadline", Value: inst.Deadline.Unix()})
	}
	return doc
}

// TestVolume_SagaMongo is TestVolume_SagaSQL on the MongoDB store.
func TestVolume_SagaMongo(t *testing.T) {
	n := livebackend.VolumeRows(t)
	db := livebackend.MongoDatabase(t, "sagavol")
	store, err := mongostore.New(db)
	require.NoError(t, err)
	coll := db.Collection(mongostore.DefaultCollectionName)
	livebackend.Measure(t, "seed", n, func() {
		livebackend.MongoBulkInsert(t, coll, n, func(i int) any { return mongoInstance(i) })
	})

	for _, def := range []string{"", volDefA} {
		// Copied from mongostore.Store.FetchRecoverable
		// (reliability/saga/storages/mongo/store.go).
		filter := bson.D{
			{Key: "status", Value: bson.M{"$in": bson.A{string(saga.StatusRunning), string(saga.StatusCompensating)}}},
			{Key: "$and", Value: bson.A{
				bson.M{"$or": bson.A{
					bson.M{"status": string(saga.StatusCompensating)},
					bson.M{"lease_owner": bson.M{"$exists": true, "$ne": ""}},
					bson.M{"deadline": bson.M{"$gt": int64(0), "$lte": volNow.Unix()}},
				}},
				bson.M{"$or": bson.A{
					bson.M{"lease_until": bson.M{"$exists": false}},
					bson.M{"lease_until": bson.M{"$lte": volNow.UnixNano()}},
				}},
			}},
		}
		if def != "" {
			filter = append(filter, bson.E{Key: "definition", Value: def})
		}
		for _, limit := range []int{0, volRecoverLimit} {
			cmd := bson.D{{Key: "find", Value: coll.Name()}, {Key: "filter", Value: filter}}
			if limit > 0 {
				cmd = append(cmd, bson.E{Key: "limit", Value: limit})
			}
			t.Logf("recoverable(%q, %d) plan indexes: %v", def, limit, livebackend.ExplainMongo(t, db, cmd))
		}
	}

	checkRecoverable(t, store, n)
	checkConcurrentUpdates(t, store, n)
}
