// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/event"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/readconcern"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver/drivertest"

	"github.com/altessa-s/go-atlas/audit"
	"github.com/altessa-s/go-atlas/audit/storages/storagetest"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// unreachableDB returns a lazily connected database pointing at a closed port,
// so any server-selecting operation fails fast without a live MongoDB.
func unreachableDB(t *testing.T) *mongo.Database {
	t.Helper()

	client, err := mongo.Connect(mongoOptions.Client().ApplyURI(
		"mongodb://127.0.0.1:1/?serverSelectionTimeoutMS=200&connectTimeoutMS=200&directConnection=true"))
	require.NoError(t, err)
	t.Cleanup(func() {
		// t.Context() is already canceled when cleanups run; best-effort disconnect.
		_ = client.Disconnect(context.WithoutCancel(t.Context()))
	})

	return client.Database("audit_unreachable")
}

func fullEvent() *audit.Event {
	return &audit.Event{
		ID:     "evt-1",
		Type:   audit.EventTypeDataChange,
		Action: audit.ActionUpdate,
		Actor: audit.Actor{
			Type:      audit.ActorTypeUser,
			ID:        "user-1",
			Name:      "Alice",
			Email:     "alice@example.com",
			IP:        "10.0.0.1",
			UserAgent: "curl/8",
			Roles:     []string{"admin", "editor"},
			Metadata:  map[string]any{"team": "core"},
		},
		Resource: audit.Resource{
			Type: "order",
			ID:   "order-1",
			Name: "Order #1",
			Path: "/orders/1",
			Changes: &audit.ResourceChanges{
				Before: map[string]any{"status": "draft"},
				After:  map[string]any{"status": "paid"},
				Fields: []string{"status"},
			},
			Attributes: map[string]any{"region": "eu"},
		},
		Result: audit.Result{
			Status:  audit.ResultStatusFailure,
			Code:    422,
			Message: "validation failed",
			Error: &audit.ResultError{
				Code:    "INVALID_STATUS",
				Message: "unknown status",
			},
		},
		Context: audit.EventContext{
			RequestID:     "req-1",
			TraceID:       "trace-1",
			SpanID:        "span-1",
			CorrelationID: "corr-1",
		},
		Service: audit.ServiceInfo{
			Name:     "orders-svc",
			Version:  "1.2.3",
			Instance: "orders-svc-0",
		},
		Timestamp: time.Date(2026, 7, 12, 10, 30, 0, 0, time.UTC),
		Duration:  150 * time.Millisecond,
		Metadata:  map[string]any{"source": "api"},
	}
}

func TestDefaultOptions(t *testing.T) {
	t.Parallel()

	o := defaultOptions()
	require.Equal(t, DefaultCollectionName, o.collectionName)
	require.Equal(t, DefaultIndexTimeout, o.indexTimeout)
	require.Zero(t, o.ttl)
}

func TestOptions(t *testing.T) {
	t.Parallel()

	name := "ptr_events"

	tests := []struct {
		name string
		opts []Option
		want options
	}{
		{
			name: "collection name string",
			opts: []Option{WithCollectionName("custom_audit")},
			want: options{collectionName: "custom_audit", indexTimeout: DefaultIndexTimeout},
		},
		{
			name: "collection name trimmed",
			opts: []Option{WithCollectionName("  spaced  ")},
			want: options{collectionName: "spaced", indexTimeout: DefaultIndexTimeout},
		},
		{
			name: "collection name empty ignored",
			opts: []Option{WithCollectionName("   ")},
			want: options{collectionName: DefaultCollectionName, indexTimeout: DefaultIndexTimeout},
		},
		{
			name: "collection name pointer",
			opts: []Option{WithCollectionName(&name)},
			want: options{collectionName: "ptr_events", indexTimeout: DefaultIndexTimeout},
		},
		{
			name: "collection name nil pointer ignored",
			opts: []Option{WithCollectionName[*string](nil)},
			want: options{collectionName: DefaultCollectionName, indexTimeout: DefaultIndexTimeout},
		},
		{
			name: "index timeout positive",
			opts: []Option{WithIndexTimeout(5 * time.Second)},
			want: options{collectionName: DefaultCollectionName, indexTimeout: 5 * time.Second},
		},
		{
			name: "index timeout zero ignored",
			opts: []Option{WithIndexTimeout(0)},
			want: options{collectionName: DefaultCollectionName, indexTimeout: DefaultIndexTimeout},
		},
		{
			name: "index timeout negative ignored",
			opts: []Option{WithIndexTimeout(-time.Second)},
			want: options{collectionName: DefaultCollectionName, indexTimeout: DefaultIndexTimeout},
		},
		{
			name: "ttl positive",
			opts: []Option{WithTTL(24 * time.Hour)},
			want: options{collectionName: DefaultCollectionName, indexTimeout: DefaultIndexTimeout, ttl: 24 * time.Hour},
		},
		{
			name: "ttl zero ignored",
			opts: []Option{WithTTL(0)},
			want: options{collectionName: DefaultCollectionName, indexTimeout: DefaultIndexTimeout},
		},
		{
			name: "ttl negative ignored",
			opts: []Option{WithTTL(-time.Hour)},
			want: options{collectionName: DefaultCollectionName, indexTimeout: DefaultIndexTimeout},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := newOptions(tc.opts...)
			require.Equal(t, tc.want, *got)
		})
	}
}

func TestBuildFilter(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)

	tests := []struct {
		name  string
		query *audit.Query
		want  bson.D
	}{
		{
			name:  "empty query",
			query: &audit.Query{},
			want:  bson.D{},
		},
		{
			name:  "start time only",
			query: &audit.Query{StartTime: &start},
			want: bson.D{{Key: fieldTimestamp, Value: bson.D{
				{Key: "$gte", Value: start},
			}}},
		},
		{
			name:  "end time only",
			query: &audit.Query{EndTime: &end},
			want: bson.D{{Key: fieldTimestamp, Value: bson.D{
				{Key: "$lte", Value: end},
			}}},
		},
		{
			name:  "time range",
			query: &audit.Query{StartTime: &start, EndTime: &end},
			want: bson.D{{Key: fieldTimestamp, Value: bson.D{
				{Key: "$gte", Value: start},
				{Key: "$lte", Value: end},
			}}},
		},
		{
			name:  "actor id",
			query: &audit.Query{ActorID: "user-1"},
			want:  bson.D{{Key: fieldActorID, Value: "user-1"}},
		},
		{
			name:  "actor type",
			query: &audit.Query{ActorType: "service"},
			want:  bson.D{{Key: fieldActorType, Value: "service"}},
		},
		{
			name:  "resource type",
			query: &audit.Query{ResourceType: "order"},
			want:  bson.D{{Key: fieldResourceType, Value: "order"}},
		},
		{
			name:  "resource id",
			query: &audit.Query{ResourceID: "order-1"},
			want:  bson.D{{Key: fieldResourceID, Value: "order-1"}},
		},
		{
			name:  "event type",
			query: &audit.Query{EventType: audit.EventTypeAuth},
			want:  bson.D{{Key: fieldType, Value: "auth"}},
		},
		{
			name:  "action",
			query: &audit.Query{Action: audit.ActionDelete},
			want:  bson.D{{Key: fieldAction, Value: "delete"}},
		},
		{
			name:  "status",
			query: &audit.Query{Status: audit.ResultStatusDenied},
			want:  bson.D{{Key: fieldResultStatus, Value: "denied"}},
		},
		{
			name:  "request id",
			query: &audit.Query{RequestID: "req-1"},
			want:  bson.D{{Key: fieldContextRequestID, Value: "req-1"}},
		},
		{
			name:  "trace id",
			query: &audit.Query{TraceID: "trace-1"},
			want:  bson.D{{Key: fieldContextTraceID, Value: "trace-1"}},
		},
		{
			name: "limit cursor and sort are not part of the filter",
			query: &audit.Query{
				Limit:     10,
				Cursor:    &audit.Cursor{ID: "x"},
				SortOrder: audit.SortOrderAsc,
			},
			want: bson.D{},
		},
		{
			name: "combined preserves append order",
			query: &audit.Query{
				StartTime: &start,
				ActorID:   "user-1",
				EventType: audit.EventTypeDataChange,
				Status:    audit.ResultStatusSuccess,
				TraceID:   "trace-1",
			},
			want: bson.D{
				{Key: fieldTimestamp, Value: bson.D{{Key: "$gte", Value: start}}},
				{Key: fieldActorID, Value: "user-1"},
				{Key: fieldType, Value: "data.change"},
				{Key: fieldResultStatus, Value: "success"},
				{Key: fieldContextTraceID, Value: "trace-1"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, buildFilter(tc.query))
		})
	}
}

func TestToModel(t *testing.T) {
	t.Parallel()

	e := fullEvent()
	m := toModel(e)

	require.Equal(t, e.ID, m.ID)
	require.Equal(t, string(e.Type), m.Type)
	require.Equal(t, string(e.Action), m.Action)
	require.Equal(t, e.Timestamp, m.Timestamp)
	require.Equal(t, int64(e.Duration), m.Duration)
	require.Equal(t, e.Metadata, m.Metadata)

	require.Equal(t, string(e.Actor.Type), m.Actor.Type)
	require.Equal(t, e.Actor.ID, m.Actor.ID)
	require.Equal(t, e.Actor.Roles, m.Actor.Roles)

	require.Equal(t, e.Resource.Type, m.Resource.Type)
	require.NotNil(t, m.Resource.Changes)
	require.Equal(t, e.Resource.Changes.Before, m.Resource.Changes.Before)
	require.Equal(t, e.Resource.Changes.After, m.Resource.Changes.After)
	require.Equal(t, e.Resource.Changes.Fields, m.Resource.Changes.Fields)

	require.Equal(t, string(e.Result.Status), m.Result.Status)
	require.Equal(t, e.Result.Code, m.Result.Code)
	require.NotNil(t, m.Result.Error)
	require.Equal(t, e.Result.Error.Code, m.Result.Error.Code)
	require.Equal(t, e.Result.Error.Message, m.Result.Error.Message)

	require.Equal(t, e.Context.RequestID, m.Context.RequestID)
	require.Equal(t, e.Service.Name, m.Service.Name)
}

func TestToModel_NilOptionalBlocks(t *testing.T) {
	t.Parallel()

	e := fullEvent()
	e.Resource.Changes = nil
	e.Result.Error = nil

	m := toModel(e)
	require.Nil(t, m.Resource.Changes)
	require.Nil(t, m.Result.Error)
}

func TestModelRoundTrip(t *testing.T) {
	t.Parallel()

	minimal := &audit.Event{
		ID:        "evt-min",
		Type:      audit.EventTypeSystem,
		Action:    audit.ActionExecute,
		Timestamp: time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC),
	}

	tests := []struct {
		name  string
		event *audit.Event
	}{
		{name: "full event", event: fullEvent()},
		{name: "minimal event", event: minimal},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.event, fromModel(toModel(tc.event)))
		})
	}
}

func TestStorage_Close(t *testing.T) {
	t.Parallel()

	s := &Storage{}
	require.NoError(t, s.Close(t.Context()))
}

func TestStorage_StoreBatch_Empty(t *testing.T) {
	t.Parallel()

	// A nil collection must not be touched when the batch is empty.
	s := &Storage{}
	require.NoError(t, s.StoreBatch(t.Context(), nil))
	require.NoError(t, s.StoreBatch(t.Context(), []*audit.Event{}))
}

func TestNew_IndexCreationError(t *testing.T) {
	t.Parallel()

	s, err := New(t.Context(), unreachableDB(t))
	require.Error(t, err)
	require.Nil(t, s)
}

func TestStorage_Store_ServerError(t *testing.T) {
	t.Parallel()

	s := &Storage{
		collection: unreachableDB(t).Collection(DefaultCollectionName),
		opts:       defaultOptions(),
	}
	require.Error(t, s.Store(t.Context(), fullEvent()))
}

func TestStorage_Query_ServerError(t *testing.T) {
	t.Parallel()

	s := &Storage{
		collection: unreachableDB(t).Collection(DefaultCollectionName),
		opts:       defaultOptions(),
	}

	var yields int
	var got error
	for e, err := range s.Query(t.Context(), nil) {
		yields++
		require.Nil(t, e)
		got = err
	}
	require.Equal(t, 1, yields)
	require.Error(t, got)
}

func TestStorage_Count_ServerError(t *testing.T) {
	t.Parallel()

	s := &Storage{
		collection: unreachableDB(t).Collection(DefaultCollectionName),
		opts:       defaultOptions(),
	}

	count, err := s.Count(t.Context(), nil)
	require.Error(t, err)
	require.Zero(t, count)
}

func TestStoreBatchNilEvent(t *testing.T) {
	t.Parallel()
	s := &Storage{}
	storagetest.NilBatchEvent(t, s)
}

func TestStoreNilEvent(t *testing.T) {
	t.Parallel()
	s := &Storage{}
	require.ErrorIs(t, s.Store(t.Context(), nil), coreerrs.ErrInvalidArgument)
}

func mockMongoDB(
	t testing.TB,
	started func(context.Context, *event.CommandStartedEvent),
	responses ...bson.D,
) *mongo.Database {
	t.Helper()
	opts := mongoOptions.Client()
	if started != nil {
		opts.SetMonitor(&event.CommandMonitor{Started: started})
	}
	deployment := drivertest.NewMockDeployment()
	if len(responses) > 0 {
		deployment.AddResponses(responses...)
	}
	opts.Deployment = deployment
	c, err := mongo.Connect(opts)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Disconnect(context.WithoutCancel(t.Context())) })
	return c.Database("audit_mock")
}

func TestNew_RejectsInvalidTTL(t *testing.T) {
	t.Parallel()

	maxValid := time.Duration(math.MaxInt32) * time.Second
	for _, tc := range []struct {
		name string
		ttl  time.Duration
	}{
		{name: "sub_second_720ns", ttl: 720 * time.Nanosecond},
		{name: "sub_second_999ms", ttl: 999 * time.Millisecond},
		{name: "overflow_int32_seconds", ttl: maxValid + time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db := mockMongoDB(t, nil,
				bson.D{
					{Key: "ok", Value: 1},
					{Key: "cursor", Value: bson.M{"id": int64(0), "ns": "audit_mock.audit_events", "firstBatch": bson.A{}}},
				},
				bson.D{{Key: "ok", Value: 1}},
			)
			s, err := New(t.Context(), db, WithTTL(tc.ttl))
			require.ErrorIs(t, err, coreerrs.ErrInvalidArgument)
			require.Nil(t, s)
		})
	}
}

func TestStoreBatch_IdempotentReplayAndWriteErrors(t *testing.T) {
	t.Parallel()

	e1 := fullEvent()
	e1.ID = "evt-1"
	e2 := fullEvent()
	e2.ID = "evt-2"
	e3 := fullEvent()
	e3.ID = "evt-3"

	t.Run("verified_replay_succeeds_unordered", func(t *testing.T) {
		t.Parallel()
		var (
			mu       sync.Mutex
			commands []bson.Raw
		)
		db := mockMongoDB(t, func(_ context.Context, ev *event.CommandStartedEvent) {
			mu.Lock()
			commands = append(commands, ev.Command)
			mu.Unlock()
		},
			bson.D{
				{Key: "ok", Value: 1},
				{Key: "n", Value: int32(1)},
				{Key: "writeErrors", Value: bson.A{
					bson.D{{Key: "index", Value: int32(0)}, {Key: "code", Value: int32(11000)}, {Key: "errmsg", Value: "E11000 secret-marker-1"}},
					bson.D{{Key: "index", Value: int32(1)}, {Key: "code", Value: int32(11000)}, {Key: "errmsg", Value: "E11000 secret-marker-2"}},
				}},
			},
			bson.D{
				{Key: "ok", Value: 1},
				{Key: "cursor", Value: bson.M{
					"id": int64(0),
					"ns": "audit_mock.audit_events",
					"firstBatch": bson.A{
						bson.D{{Key: "_id", Value: "evt-1"}},
						bson.D{{Key: "_id", Value: "evt-2"}},
					},
				}},
			},
		)
		s := &Storage{collection: db.Collection(DefaultCollectionName), opts: defaultOptions()}
		require.NoError(t, s.StoreBatch(t.Context(), []*audit.Event{e1, e2, e3}))

		mu.Lock()
		defer mu.Unlock()
		require.NotEmpty(t, commands)
		ordered, ok := commands[0].Lookup("ordered").BooleanOK()
		require.True(t, ok)
		require.False(t, ordered, "StoreBatch must execute unordered InsertMany")
	})

	t.Run("duplicate_on_other_unique_index_rejected_without_marker", func(t *testing.T) {
		t.Parallel()
		db := mockMongoDB(t, nil,
			bson.D{
				{Key: "ok", Value: 1},
				{Key: "n", Value: int32(0)},
				{Key: "writeErrors", Value: bson.A{
					bson.D{{Key: "index", Value: int32(0)}, {Key: "code", Value: int32(11000)}, {Key: "errmsg", Value: "E11000 replay secret-marker"}},
					bson.D{{Key: "index", Value: int32(1)}, {Key: "code", Value: int32(11000)}, {Key: "errmsg", Value: "E11000 secondary index secret-marker"}},
				}},
			},
			bson.D{
				{Key: "ok", Value: 1},
				{Key: "cursor", Value: bson.M{
					"id": int64(0),
					"ns": "audit_mock.audit_events",
					"firstBatch": bson.A{
						bson.D{{Key: "_id", Value: "evt-1"}},
					},
				}},
			},
		)
		s := &Storage{collection: db.Collection(DefaultCollectionName), opts: defaultOptions()}
		err := s.StoreBatch(t.Context(), []*audit.Event{e1, e2})
		require.ErrorIs(t, err, coreerrs.ErrConflict)
		require.NotContains(t, err.Error(), "secret-marker")
	})

	t.Run("mixed_validation_and_duplicate_failure_rejected_without_marker", func(t *testing.T) {
		t.Parallel()
		db := mockMongoDB(t, nil,
			bson.D{
				{Key: "ok", Value: 1},
				{Key: "n", Value: int32(0)},
				{Key: "writeErrors", Value: bson.A{
					bson.D{{Key: "index", Value: int32(0)}, {Key: "code", Value: int32(11000)}, {Key: "errmsg", Value: "E11000 secret-marker"}},
					bson.D{{Key: "index", Value: int32(1)}, {Key: "code", Value: int32(121)}, {Key: "errmsg", Value: "Document failed validation secret-marker"}},
				}},
			},
		)
		s := &Storage{collection: db.Collection(DefaultCollectionName), opts: defaultOptions()}
		err := s.StoreBatch(t.Context(), []*audit.Event{e1, e2})
		require.ErrorIs(t, err, coreerrs.ErrFailedPrecondition)
		require.NotContains(t, err.Error(), "secret-marker")
	})

	t.Run("write_concern_error_always_propagates_without_marker", func(t *testing.T) {
		t.Parallel()
		db := mockMongoDB(t, nil,
			bson.D{
				{Key: "ok", Value: 1},
				{Key: "n", Value: int32(1)},
				{Key: "writeConcernError", Value: bson.D{
					{Key: "code", Value: int32(64)},
					{Key: "errmsg", Value: "waiting for replication timed out secret-marker"},
				}},
			},
		)
		s := &Storage{collection: db.Collection(DefaultCollectionName), opts: defaultOptions()}
		err := s.StoreBatch(t.Context(), []*audit.Event{e1})
		require.ErrorIs(t, err, coreerrs.ErrUnavailable)
		require.NotContains(t, err.Error(), "secret-marker")
	})

	t.Run("split_wire_batch_preserves_first_batch_write_concern_error", func(t *testing.T) {
		t.Parallel()
		// Three ~16 MB documents exceed MongoDB's 48 MB OP_MSG limit, forcing a
		// single InsertMany to split into two wire batches on MockDeployment.
		pad := strings.Repeat("x", 16_000_000)
		b1 := fullEvent()
		b1.ID = "evt-b1"
		b1.Metadata = map[string]any{"pad": pad}
		b2 := fullEvent()
		b2.ID = "evt-b2"
		b2.Metadata = map[string]any{"pad": pad}
		b3 := fullEvent()
		b3.ID = "evt-b3"
		b3.Metadata = map[string]any{"pad": pad}

		db := mockMongoDB(t, nil,
			// First wire batch: replication write-concern timeout.
			bson.D{
				{Key: "ok", Value: 1},
				{Key: "n", Value: int32(1)},
				{Key: "writeConcernError", Value: bson.D{
					{Key: "code", Value: int32(64)},
					{Key: "errmsg", Value: "waiting for replication timed out secret-marker-wce"},
				}},
			},
			// Second wire batch: duplicate key only, no writeConcernError.
			bson.D{
				{Key: "ok", Value: 1},
				{Key: "n", Value: int32(0)},
				{Key: "writeErrors", Value: bson.A{
					bson.D{{Key: "index", Value: int32(0)}, {Key: "code", Value: int32(11000)}, {Key: "errmsg", Value: "E11000 secret-marker-dup"}},
				}},
			},
			// Third response: valid ok:1 with cursor so an unsplit InsertMany that
			// dropped the write-concern error and ran Find would return nil.
			bson.D{
				{Key: "ok", Value: 1},
				{Key: "n", Value: int32(1)},
				{Key: "cursor", Value: bson.M{
					"id":         int64(0),
					"ns":         "audit_mock.audit_events",
					"firstBatch": bson.A{bson.D{{Key: "_id", Value: "evt-b3"}}},
				}},
			},
		)
		// Third response: valid ok:1 with cursor so an unsplit InsertMany that
		// dropped the write-concern error and ran Find would return nil.
		s := &Storage{collection: db.Collection(DefaultCollectionName), opts: defaultOptions()}
		err := s.StoreBatch(t.Context(), []*audit.Event{b1, b2, b3})
		require.ErrorIs(t, err, coreerrs.ErrUnavailable)
		require.NotContains(t, err.Error(), "secret-marker")
	})

	t.Run("count_split_preserves_first_subbatch_write_concern_error", func(t *testing.T) {
		t.Parallel()
		events := make([]*audit.Event, maxInsertBatchDocs+1)
		for i := range events {
			ev := fullEvent()
			ev.ID = "evt-cnt"
			events[i] = ev
		}
		db := mockMongoDB(t, nil,
			bson.D{
				{Key: "ok", Value: 1},
				{Key: "n", Value: int32(maxInsertBatchDocs)},
				{Key: "writeConcernError", Value: bson.D{
					{Key: "code", Value: int32(64)},
					{Key: "errmsg", Value: "replication timeout secret-marker-cnt"},
				}},
			},
			bson.D{
				{Key: "ok", Value: 1},
				{Key: "n", Value: int32(0)},
				{Key: "writeErrors", Value: bson.A{
					bson.D{{Key: "index", Value: int32(0)}, {Key: "code", Value: int32(11000)}, {Key: "errmsg", Value: "E11000 secret-marker-cnt"}},
				}},
			},
		)
		s := &Storage{collection: db.Collection(DefaultCollectionName), opts: defaultOptions()}
		err := s.StoreBatch(t.Context(), events)
		require.ErrorIs(t, err, coreerrs.ErrUnavailable)
		require.NotContains(t, err.Error(), "secret-marker")
	})

	t.Run("verification_find_uses_primary_and_majority_read_concern", func(t *testing.T) {
		t.Parallel()
		var (
			mu      sync.Mutex
			findCmd bson.Raw
		)
		baseDB := mockMongoDB(t, func(_ context.Context, ev *event.CommandStartedEvent) {
			if ev.CommandName == "find" {
				mu.Lock()
				findCmd = ev.Command
				mu.Unlock()
			}
		},
			bson.D{
				{Key: "ok", Value: 1},
				{Key: "n", Value: int32(0)},
				{Key: "writeErrors", Value: bson.A{
					bson.D{{Key: "index", Value: int32(0)}, {Key: "code", Value: int32(11000)}, {Key: "errmsg", Value: "E11000"}},
				}},
			},
			bson.D{
				{Key: "ok", Value: 1},
				{Key: "cursor", Value: bson.M{
					"id":         int64(0),
					"ns":         "audit_mock.audit_events",
					"firstBatch": bson.A{bson.D{{Key: "_id", Value: "evt-1"}}},
				}},
			},
		)
		secondaryDB := baseDB.Client().Database("audit_mock",
			mongoOptions.Database().
				SetReadPreference(readpref.Secondary()).
				SetReadConcern(readconcern.Local()),
		)
		s := &Storage{collection: secondaryDB.Collection(DefaultCollectionName), opts: defaultOptions()}
		require.NoError(t, s.StoreBatch(t.Context(), []*audit.Event{e1}))

		mu.Lock()
		defer mu.Unlock()
		require.NotEmpty(t, findCmd, "verification find must run")
		rcLevel, ok := findCmd.Lookup("readConcern", "level").StringValueOK()
		require.True(t, ok, "verification find must send explicit readConcern")
		require.Equal(t, "majority", rcLevel)
		if rpMode, hasRP := findCmd.Lookup("$readPreference", "mode").StringValueOK(); hasRP {
			require.NotEqual(t, "secondary", rpMode, "verification find must not read from secondary")
			require.NotEqual(t, "secondaryPreferred", rpMode, "verification find must not prefer secondary")
		}
	})

	t.Run("command_and_verification_errors_redact_server_text", func(t *testing.T) {
		t.Parallel()

		t.Run("insert_command_error", func(t *testing.T) {
			t.Parallel()
			db := mockMongoDB(t, nil,
				bson.D{
					{Key: "ok", Value: 0},
					{Key: "code", Value: int32(13)},
					{Key: "codeName", Value: "Unauthorized"},
					{Key: "errmsg", Value: "not authorized secret-marker-insert"},
				},
			)
			s := &Storage{collection: db.Collection(DefaultCollectionName), opts: defaultOptions()}
			err := s.StoreBatch(t.Context(), []*audit.Event{e1})
			require.Error(t, err)
			require.NotContains(t, err.Error(), "secret-marker")
		})

		t.Run("verification_find_command_error", func(t *testing.T) {
			t.Parallel()
			db := mockMongoDB(t, nil,
				bson.D{
					{Key: "ok", Value: 1},
					{Key: "n", Value: int32(0)},
					{Key: "writeErrors", Value: bson.A{
						bson.D{{Key: "index", Value: int32(0)}, {Key: "code", Value: int32(11000)}, {Key: "errmsg", Value: "E11000"}},
					}},
				},
				bson.D{
					{Key: "ok", Value: 0},
					{Key: "code", Value: int32(13)},
					{Key: "codeName", Value: "Unauthorized"},
					{Key: "errmsg", Value: "not authorized on filter {_id: {$in: [\"secret-marker-find\"]}}"},
				},
			)
			s := &Storage{collection: db.Collection(DefaultCollectionName), opts: defaultOptions()}
			err := s.StoreBatch(t.Context(), []*audit.Event{e1})
			require.Error(t, err)
			require.NotContains(t, err.Error(), "secret-marker")
		})

		t.Run("verification_getmore_command_error", func(t *testing.T) {
			t.Parallel()
			db := mockMongoDB(t, nil,
				bson.D{
					{Key: "ok", Value: 1},
					{Key: "n", Value: int32(0)},
					{Key: "writeErrors", Value: bson.A{
						bson.D{{Key: "index", Value: int32(0)}, {Key: "code", Value: int32(11000)}, {Key: "errmsg", Value: "E11000"}},
						bson.D{{Key: "index", Value: int32(1)}, {Key: "code", Value: int32(11000)}, {Key: "errmsg", Value: "E11000"}},
					}},
				},
				bson.D{
					{Key: "ok", Value: 1},
					{Key: "cursor", Value: bson.M{
						"id":         int64(77),
						"ns":         "audit_mock.audit_events",
						"firstBatch": bson.A{bson.D{{Key: "_id", Value: "evt-1"}}},
					}},
				},
				bson.D{
					{Key: "ok", Value: 0},
					{Key: "code", Value: int32(43)},
					{Key: "codeName", Value: "CursorNotFound"},
					{Key: "errmsg", Value: "cursor 77 not found secret-marker-getmore"},
				},
			)
			s := &Storage{collection: db.Collection(DefaultCollectionName), opts: defaultOptions()}
			err := s.StoreBatch(t.Context(), []*audit.Event{e1, e2})
			require.Error(t, err)
			require.NotContains(t, err.Error(), "secret-marker")
		})
	})
}

func TestStore_IdempotentReplayAndWriteErrors(t *testing.T) {
	t.Parallel()

	e1 := fullEvent()
	e1.ID = "evt-single"

	t.Run("verified_replay_succeeds", func(t *testing.T) {
		t.Parallel()
		db := mockMongoDB(t, nil,
			bson.D{
				{Key: "ok", Value: 1},
				{Key: "n", Value: int32(0)},
				{Key: "writeErrors", Value: bson.A{
					bson.D{{Key: "index", Value: int32(0)}, {Key: "code", Value: int32(11000)}, {Key: "errmsg", Value: "E11000 secret-marker"}},
				}},
			},
			bson.D{
				{Key: "ok", Value: 1},
				{Key: "cursor", Value: bson.M{
					"id":         int64(0),
					"ns":         "audit_mock.audit_events",
					"firstBatch": bson.A{bson.D{{Key: "_id", Value: "evt-single"}}},
				}},
			},
		)
		s := &Storage{collection: db.Collection(DefaultCollectionName), opts: defaultOptions()}
		require.NoError(t, s.Store(t.Context(), e1))
	})

	t.Run("duplicate_on_other_unique_index_rejected_without_marker", func(t *testing.T) {
		t.Parallel()
		db := mockMongoDB(t, nil,
			bson.D{
				{Key: "ok", Value: 1},
				{Key: "n", Value: int32(0)},
				{Key: "writeErrors", Value: bson.A{
					bson.D{{Key: "index", Value: int32(0)}, {Key: "code", Value: int32(11000)}, {Key: "errmsg", Value: "E11000 secret-marker"}},
				}},
			},
			bson.D{
				{Key: "ok", Value: 1},
				{Key: "cursor", Value: bson.M{
					"id":         int64(0),
					"ns":         "audit_mock.audit_events",
					"firstBatch": bson.A{},
				}},
			},
		)
		s := &Storage{collection: db.Collection(DefaultCollectionName), opts: defaultOptions()}
		err := s.Store(t.Context(), e1)
		require.ErrorIs(t, err, coreerrs.ErrConflict)
		require.NotContains(t, err.Error(), "secret-marker")
	})
}

type probingErr struct{}

func (probingErr) Error() string { return "secret-probe-text" }
func (probingErr) Is(error) bool { return true }

func TestSafeMongoError(t *testing.T) {
	t.Parallel()

	require.NoError(t, safeMongoError("store audit events", nil))

	canceled := safeMongoError("store audit events", fmt.Errorf("secret-wrapper: %w", context.Canceled))
	require.ErrorIs(t, canceled, context.Canceled)
	require.NotErrorIs(t, canceled, coreerrs.ErrUnavailable)
	require.NotContains(t, canceled.Error(), "secret-wrapper")

	deadline := safeMongoError("store audit events", fmt.Errorf("secret-wrapper: %w", context.DeadlineExceeded))
	require.ErrorIs(t, deadline, context.DeadlineExceeded)
	require.ErrorIs(t, deadline, coreerrs.ErrUnavailable)
	require.NotContains(t, deadline.Error(), "secret-wrapper")

	netErr := safeMongoError("store audit events", &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("secret-host:27017")})
	require.ErrorIs(t, netErr, coreerrs.ErrUnavailable)
	require.NotContains(t, netErr.Error(), "secret-host")

	probe := safeMongoError("store audit events", probingErr{})
	require.NotErrorIs(t, probe, context.Canceled, "custom Is methods must not be consulted")
	require.NotContains(t, probe.Error(), "secret-probe-text")
}
