// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/data/audit"

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
		_ = client.Disconnect(context.Background())
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
			name: "limit offset and sort are not part of the filter",
			query: &audit.Query{
				Limit:     10,
				Offset:    5,
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

	s, err := New(unreachableDB(t))
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
	for e, err := range s.Query(t.Context(), &audit.Query{}) {
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

	count, err := s.Count(t.Context(), &audit.Query{})
	require.Error(t, err)
	require.Zero(t, count)
}
