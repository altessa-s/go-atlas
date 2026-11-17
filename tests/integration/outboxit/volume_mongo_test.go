//go:build volume

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outboxit_test

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/reliability/outbox"
	"github.com/altessa-s/go-atlas/tests/integration/internal/livebackend"

	outboxmongo "github.com/altessa-s/go-atlas/reliability/outbox/storages/mongo"
)

func newVolumeMongo(t testing.TB, seed volumeSeed) (*mongo.Collection, volumeBackend) {
	t.Helper()
	db := livebackend.MongoDatabase(t, "obvol")
	coll := db.Collection(livebackend.UniqueName(t, "obvol"))
	require.NoError(t, db.CreateCollection(t.Context(), coll.Name()))
	// The constructor creates the production indexes; no test-only indexes.
	store, err := outboxmongo.NewWithCollectionOptions(coll, outboxmongo.WithContext(t.Context()))
	require.NoError(t, err)
	livebackend.Measure(t, "seed MongoDB outbox", seed.total, func() {
		offset := 0
		for _, g := range seed.groups {
			livebackend.MongoBulkInsert(t, coll, g.n, func(i int) any {
				ev := volumeEvent(t, seed, g, offset, i)
				var seq bson.ObjectID
				binary.BigEndian.PutUint64(seq[4:], uint64(offset+i+1))
				// Matches reliability/outbox/storages/mongo/entity.go:event:
				// payload is BSON binary in "event", topic is a string, dates
				// are BSON Date, and optional dates are absent when unset.
				doc := bson.M{
					"_id": ev.Id, "status": string(ev.Status), "topic": ev.Key,
					"event": ev.Payload, "created_at": ev.CreatedAt,
					"attempts": ev.Attempts, "seq": seq,
				}
				if !ev.PublishedAt.IsZero() {
					doc["published_at"] = ev.PublishedAt
				}
				if !ev.ExpiresAt.IsZero() {
					doc["expires_at"] = ev.ExpiresAt
				}
				if ev.LastError != nil {
					doc["error"] = *ev.LastError
					doc["last_attempt_on"] = ev.LastAttemptOn
				}
				return doc
			})
			offset += g.n
		}
	})
	return coll, volumeBackend{store: store, count: func(t testing.TB, group string) int64 {
		t.Helper()
		filter := bson.M{}
		if group != "" {
			filter["_id"] = bson.M{"$gte": group + "-", "$lt": group + "."}
		}
		n, err := coll.CountDocuments(t.Context(), filter)
		require.NoError(t, err)
		return n
	}}
}

func volumeMongoExplain(t *testing.T) {
	seed := newVolumeSeed(t)
	coll, _ := newVolumeMongo(t, seed)
	// One subtest per query, so a full scan does not hide later hot queries.
	for _, q := range volumeMongoQueries(t, coll.Name()) {
		t.Run(q.name, func(t *testing.T) {
			t.Logf("%s: indexes %v", q.name, livebackend.ExplainMongo(t, coll.Database(), q.command))
		})
	}
}

type volumeMongoQuery struct {
	name    string
	command bson.D
}

func volumeMongoQueries(t testing.TB, collection string) []volumeMongoQuery {
	t.Helper()
	// Query shapes copied from reliability/outbox/storages/mongo/store.go:
	// FetchUnprocessedEvents, UnlockStuckEvents, ExpireEvents,
	// DeleteProcessedEvents and Stats. Keep $$NOW and $expr intact: replacing
	// them with client dates could conceal an unindexable production query.
	notExpired := bson.M{"$or": bson.A{
		bson.M{"expires_at": bson.M{"$exists": false}},
		bson.M{"$expr": bson.M{"$gt": bson.A{"$expires_at", "$$NOW"}}},
	}}
	backoffElapsed := bson.M{"$or": bson.A{
		bson.M{"next_attempt_at": bson.M{"$exists": false}},
		bson.M{"next_attempt_at": nil},
		bson.M{"$expr": bson.M{"$lte": bson.A{"$next_attempt_at", "$$NOW"}}},
	}}
	pending := bson.M{"status": outbox.StatusPending}
	failed := bson.M{"$and": bson.A{bson.M{"status": outbox.StatusFailed}, backoffElapsed}}
	ready := bson.M{"$or": bson.A{pending, failed}}
	sort := bson.D{{Key: "created_at", Value: 1}, {Key: "seq", Value: 1}}
	queries := make([]volumeMongoQuery, 0, 7)
	// The combined predicate is the actual claim. Explain its two eligibility
	// branches too, so one indexed branch cannot conceal a scan in the other.
	for _, branch := range []struct {
		name   string
		filter bson.M
	}{{"fetch_pending", pending}, {"fetch_failed", failed}, {"fetch_combined", ready}} {
		queries = append(queries, volumeMongoQuery{branch.name, bson.D{
			{Key: "find", Value: collection},
			{Key: "filter", Value: bson.M{"$and": bson.A{branch.filter, notExpired}}},
			{Key: "sort", Value: sort}, {Key: "limit", Value: volumeBatchSize},
		}})
	}
	unlockFilter := bson.M{
		"status": outbox.StatusInProgress,
		"$expr":  bson.M{"$lt": bson.A{"$locked_on", bson.M{"$subtract": bson.A{"$$NOW", (5 * time.Minute).Milliseconds()}}}},
	}
	expireFilter := bson.M{
		"status":     bson.M{"$in": bson.A{outbox.StatusPending, outbox.StatusFailed}},
		"expires_at": bson.M{"$exists": true},
		"$expr":      bson.M{"$lte": bson.A{"$expires_at", "$$NOW"}},
	}
	// Production has no expires_at index. With most rows pending, this sweep
	// may choose COLLSCAN; keep that choice visible as a strict test failure.
	for _, sweep := range []struct {
		name   string
		filter bson.M
		set    bson.M
	}{
		{"unlock", unlockFilter, bson.M{"status": outbox.StatusPending, "locked_on": nil, "lock_token": nil, "next_attempt_at": nil}},
		{"expire", expireFilter, bson.M{"status": outbox.StatusExpired, "published_at": "$$NOW", "locked_on": nil, "lock_token": nil, "next_attempt_at": nil}},
	} {
		queries = append(queries, volumeMongoQuery{sweep.name, bson.D{
			{Key: "update", Value: collection},
			{Key: "updates", Value: bson.A{bson.M{
				"q": sweep.filter, "u": mongo.Pipeline{bson.D{{Key: "$set", Value: sweep.set}}}, "multi": true,
			}}},
		}})
	}
	queries = append(queries, volumeMongoQuery{"delete_processed", bson.D{
		{Key: "delete", Value: collection},
		{Key: "deletes", Value: bson.A{bson.M{"q": bson.M{
			"status": bson.M{"$in": bson.A{outbox.StatusSent, outbox.StatusSkipped, outbox.StatusExpired}},
			"$expr":  bson.M{"$lt": bson.A{"$published_at", bson.M{"$subtract": bson.A{"$$NOW", volumeLifetime.Milliseconds()}}}},
		}, "limit": 0}}},
	}})
	counted := bson.A{outbox.StatusInProgress, outbox.StatusPending, outbox.StatusFailed, outbox.StatusMaxAttemptReached, outbox.StatusRejected}
	// The explain of an aggregate nests its query planner under the pipeline
	// stages, which livebackend.ExplainMongo does not read, so the stats
	// pipeline is explained through its leading $match — the only stage that
	// reads the collection — as a find with the same filter.
	queries = append(queries, volumeMongoQuery{"stats", bson.D{
		{Key: "find", Value: collection}, {Key: "filter", Value: bson.M{"status": bson.M{"$in": counted}}},
	}})
	return queries
}
