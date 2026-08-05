// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo_test

import (
	"context"
	"iter"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/data/audit"

	auditmongo "github.com/altessa-s/go-atlas/data/audit/storages/mongo"
	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// indexSpec is the shape of documents returned by ListIndexes.
type indexSpec struct {
	Name               string `bson:"name"`
	Key                bson.D `bson:"key"`
	Sparse             bool   `bson:"sparse"`
	ExpireAfterSeconds *int32 `bson:"expireAfterSeconds"`
}

// mongoURI returns the MongoDB connection string, honoring MONGO_URI for CI.
func mongoURI() string {
	if uri := os.Getenv("MONGO_URI"); uri != "" {
		return uri
	}
	return "mongodb://localhost:27017"
}

// newIT connects to a live MongoDB, skipping the test when none is reachable.
// Each call gets its own throwaway database (dropped on cleanup) and a Storage
// bound to it.
func newIT(t *testing.T, opts ...auditmongo.Option) (*auditmongo.Storage, *mongo.Database) {
	t.Helper()

	client, err := mongo.Connect(mongoOptions.Client().ApplyURI(mongoURI()))
	if err != nil {
		t.Skipf("mongodb not available: %v", err)
	}
	if err := client.Ping(t.Context(), nil); err != nil {
		_ = client.Disconnect(context.Background())
		t.Skipf("mongodb not reachable at %s: %v", mongoURI(), err)
	}

	dbName := "audit_it_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	db := client.Database(dbName)
	t.Cleanup(func() {
		_ = db.Drop(context.Background())
		_ = client.Disconnect(context.Background())
	})

	storage, err := auditmongo.New(db, opts...)
	if err != nil {
		t.Skipf("mongodb storage setup failed (server not usable for tests): %v", err)
	}
	return storage, db
}

// listIndexes returns all indexes of coll keyed by their auto-generated name.
func listIndexes(t *testing.T, coll *mongo.Collection) map[string]indexSpec {
	t.Helper()

	cursor, err := coll.Indexes().List(t.Context())
	require.NoError(t, err)

	var specs []indexSpec
	require.NoError(t, cursor.All(t.Context(), &specs))

	byName := make(map[string]indexSpec, len(specs))
	for _, s := range specs {
		byName[s.Name] = s
	}
	return byName
}

// collectEvents drains the Query iterator, failing the test on any yielded error.
func collectEvents(t *testing.T, seq iter.Seq2[*audit.Event, error]) []*audit.Event {
	t.Helper()

	var out []*audit.Event
	for e, err := range seq {
		require.NoError(t, err)
		out = append(out, e)
	}
	return out
}

// testEvent builds a stored-form event: timestamps truncated to millisecond
// (BSON DateTime precision) so round-trip equality holds.
func testEvent(id, actorID string, ts time.Time) *audit.Event {
	return &audit.Event{
		ID:     id,
		Type:   audit.EventTypeDataChange,
		Action: audit.ActionUpdate,
		Actor: audit.Actor{
			Type: audit.ActorTypeUser,
			ID:   actorID,
		},
		Resource: audit.Resource{
			Type: "order",
			ID:   "order-" + id,
		},
		Result: audit.Result{
			Status: audit.ResultStatusSuccess,
		},
		Service: audit.ServiceInfo{
			Name: "orders-svc",
		},
		Timestamp: ts.UTC().Truncate(time.Millisecond),
	}
}

func eventIDs(events []*audit.Event) []string {
	if len(events) == 0 {
		return nil
	}
	ids := make([]string, len(events))
	for i, e := range events {
		ids[i] = e.ID
	}
	return ids
}

func TestIntegration_New_CreatesIndexes(t *testing.T) {
	t.Parallel()

	_, db := newIT(t)
	indexes := listIndexes(t, db.Collection(auditmongo.DefaultCollectionName))

	// _id_ plus the four indexes created by New.
	require.Len(t, indexes, 5)

	tsType, ok := indexes["timestamp_-1_type_1"]
	require.True(t, ok, "timestamp+type index missing: %v", indexes)
	require.Equal(t, bson.D{
		{Key: "timestamp", Value: int32(-1)},
		{Key: "type", Value: int32(1)},
	}, tsType.Key)

	actor, ok := indexes["actor.id_1_timestamp_-1"]
	require.True(t, ok, "actor index missing: %v", indexes)
	require.Equal(t, bson.D{
		{Key: "actor.id", Value: int32(1)},
		{Key: "timestamp", Value: int32(-1)},
	}, actor.Key)

	resource, ok := indexes["resource.type_1_resource.id_1_timestamp_-1"]
	require.True(t, ok, "resource index missing: %v", indexes)
	require.Equal(t, bson.D{
		{Key: "resource.type", Value: int32(1)},
		{Key: "resource.id", Value: int32(1)},
		{Key: "timestamp", Value: int32(-1)},
	}, resource.Key)

	request, ok := indexes["context.request_id_1"]
	require.True(t, ok, "request_id index missing: %v", indexes)
	require.Equal(t, bson.D{{Key: "context.request_id", Value: int32(1)}}, request.Key)
	require.True(t, request.Sparse, "request_id index must be sparse")
	require.Nil(t, request.ExpireAfterSeconds)
}

func TestIntegration_New_TTLIndex(t *testing.T) {
	t.Parallel()

	_, db := newIT(t, auditmongo.WithTTL(24*time.Hour))
	indexes := listIndexes(t, db.Collection(auditmongo.DefaultCollectionName))

	require.Len(t, indexes, 6)

	ttl, ok := indexes["timestamp_1"]
	require.True(t, ok, "TTL index missing: %v", indexes)
	require.Equal(t, bson.D{{Key: "timestamp", Value: int32(1)}}, ttl.Key)
	require.NotNil(t, ttl.ExpireAfterSeconds)
	require.Equal(t, int32(24*60*60), *ttl.ExpireAfterSeconds)
}

func TestIntegration_New_CustomCollectionName(t *testing.T) {
	t.Parallel()

	storage, db := newIT(t, auditmongo.WithCollectionName("custom_audit"))
	ctx := t.Context()

	require.NoError(t, storage.Store(ctx, testEvent("evt-1", "user-1", time.Now())))

	count, err := db.Collection("custom_audit").CountDocuments(ctx, bson.D{})
	require.NoError(t, err)
	require.Equal(t, int64(1), count)
}

func TestIntegration_Store_QueryRoundTrip(t *testing.T) {
	t.Parallel()

	storage, _ := newIT(t)
	ctx := t.Context()

	event := testEvent("evt-full", "user-1", time.Now())
	event.Actor.Name = "Alice"
	event.Actor.Roles = []string{"admin"}
	event.Actor.Metadata = map[string]any{"team": "core"}
	event.Resource.Changes = &audit.ResourceChanges{
		Before: map[string]any{"status": "draft"},
		After:  map[string]any{"status": "paid"},
		Fields: []string{"status"},
	}
	event.Result.Status = audit.ResultStatusError
	event.Result.Code = 500
	event.Result.Error = &audit.ResultError{Code: "E1", Message: "boom"}
	event.Context = audit.EventContext{
		RequestID:     "req-1",
		TraceID:       "trace-1",
		SpanID:        "span-1",
		CorrelationID: "corr-1",
	}
	event.Duration = 42 * time.Millisecond
	event.Metadata = map[string]any{"source": "api"}

	require.NoError(t, storage.Store(ctx, event))

	got := collectEvents(t, storage.Query(ctx, &audit.Query{RequestID: "req-1"}))
	require.Len(t, got, 1)
	require.Equal(t, event, got[0])
}

func TestIntegration_StoreBatch_CountAndFilters(t *testing.T) {
	t.Parallel()

	storage, _ := newIT(t)
	ctx := t.Context()

	base := time.Now()
	e1 := testEvent("evt-1", "user-1", base)
	e2 := testEvent("evt-2", "user-1", base.Add(time.Second))
	e2.Result.Status = audit.ResultStatusDenied
	e3 := testEvent("evt-3", "user-2", base.Add(2*time.Second))
	e3.Type = audit.EventTypeAuth
	e3.Action = audit.ActionLogin

	require.NoError(t, storage.StoreBatch(ctx, []*audit.Event{e1, e2, e3}))

	total, err := storage.Count(ctx, &audit.Query{})
	require.NoError(t, err)
	require.Equal(t, int64(3), total)

	tests := []struct {
		name  string
		query *audit.Query
		want  []string
	}{
		{
			name:  "by actor id",
			query: &audit.Query{ActorID: "user-1"},
			want:  []string{"evt-2", "evt-1"},
		},
		{
			name:  "by status",
			query: &audit.Query{Status: audit.ResultStatusDenied},
			want:  []string{"evt-2"},
		},
		{
			name:  "by event type and action",
			query: &audit.Query{EventType: audit.EventTypeAuth, Action: audit.ActionLogin},
			want:  []string{"evt-3"},
		},
		{
			name:  "by time range",
			query: &audit.Query{StartTime: &e2.Timestamp, EndTime: &e3.Timestamp},
			want:  []string{"evt-3", "evt-2"},
		},
		{
			name:  "no match",
			query: &audit.Query{ActorID: "nobody"},
			want:  nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := collectEvents(t, storage.Query(ctx, tc.query))
			require.Equal(t, tc.want, eventIDs(got))

			count, err := storage.Count(ctx, tc.query)
			require.NoError(t, err)
			require.Equal(t, int64(len(tc.want)), count)
		})
	}
}

func TestIntegration_Query_SortLimitOffset(t *testing.T) {
	t.Parallel()

	storage, _ := newIT(t)
	ctx := t.Context()

	base := time.Now()
	events := []*audit.Event{
		testEvent("evt-1", "user-1", base),
		testEvent("evt-2", "user-1", base.Add(time.Second)),
		testEvent("evt-3", "user-1", base.Add(2*time.Second)),
	}
	require.NoError(t, storage.StoreBatch(ctx, events))

	tests := []struct {
		name  string
		query *audit.Query
		want  []string
	}{
		{
			name:  "default sort is newest first",
			query: &audit.Query{},
			want:  []string{"evt-3", "evt-2", "evt-1"},
		},
		{
			name:  "ascending sort",
			query: &audit.Query{SortOrder: audit.SortOrderAsc},
			want:  []string{"evt-1", "evt-2", "evt-3"},
		},
		{
			name:  "limit",
			query: &audit.Query{Limit: 2},
			want:  []string{"evt-3", "evt-2"},
		},
		{
			name:  "offset",
			query: &audit.Query{Offset: 1, SortOrder: audit.SortOrderAsc},
			want:  []string{"evt-2", "evt-3"},
		},
		{
			name:  "limit and offset",
			query: &audit.Query{Limit: 1, Offset: 1, SortOrder: audit.SortOrderAsc},
			want:  []string{"evt-2"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := collectEvents(t, storage.Query(ctx, tc.query))
			require.Equal(t, tc.want, eventIDs(got))
		})
	}
}

func TestIntegration_Query_EarlyBreak(t *testing.T) {
	t.Parallel()

	storage, _ := newIT(t)
	ctx := t.Context()

	base := time.Now()
	require.NoError(t, storage.StoreBatch(ctx, []*audit.Event{
		testEvent("evt-1", "user-1", base),
		testEvent("evt-2", "user-1", base.Add(time.Second)),
	}))

	var seen int
	for _, err := range storage.Query(ctx, &audit.Query{}) {
		require.NoError(t, err)
		seen++
		break
	}
	require.Equal(t, 1, seen)
}
