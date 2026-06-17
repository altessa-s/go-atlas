// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo_test

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/data/saga"

	sagaerrs "github.com/altessa-s/go-atlas/data/saga/errs"
	mongostore "github.com/altessa-s/go-atlas/data/saga/storages/mongo"
	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// mongoURI returns the MongoDB connection string, honoring MONGO_URI for CI.
func mongoURI() string {
	if uri := os.Getenv("MONGO_URI"); uri != "" {
		return uri
	}
	return "mongodb://localhost:27017"
}

// newIntegrationStore connects to a live MongoDB, skipping the test when none is
// reachable. Each call gets its own throwaway database, dropped on cleanup.
func newIntegrationStore(t *testing.T) *mongostore.Store {
	t.Helper()

	client, err := mongo.Connect(mongoOptions.Client().ApplyURI(mongoURI()))
	if err != nil {
		t.Skipf("mongodb not available: %v", err)
	}
	// Cleanups run after t.Context() is canceled, so they use a fresh context.
	if err := client.Ping(context.Background(), nil); err != nil {
		_ = client.Disconnect(context.Background())
		t.Skipf("mongodb not reachable at %s: %v", mongoURI(), err)
	}

	dbName := "saga_it_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	db := client.Database(dbName)
	t.Cleanup(func() {
		_ = db.Drop(context.Background())
		_ = client.Disconnect(context.Background())
	})

	// A reachable server may still reject us (auth, permissions): treat store
	// setup failure as "no usable server" and skip rather than fail.
	store, err := mongostore.New(db)
	if err != nil {
		t.Skipf("mongodb store setup failed (server not usable for tests): %v", err)
	}
	return store
}

// itInstance builds a saga instance for the integration tests.
func itInstance(id string, status saga.Status, deadline time.Time) *saga.Instance {
	now := time.Now().UTC().Truncate(time.Second)
	return &saga.Instance{
		ID:         id,
		Definition: "place-order",
		Status:     status,
		CreatedAt:  now,
		UpdatedAt:  now,
		Deadline:   deadline,
		Data:       []byte(`{"n":1}`),
	}
}

func TestIntegrationCRUD(t *testing.T) {
	t.Parallel()
	s := newIntegrationStore(t)
	ctx := t.Context()

	inst := itInstance("a", saga.StatusRunning, time.Time{})
	require.NoError(t, s.Create(ctx, inst))

	got, err := s.Get(ctx, "a")
	require.NoError(t, err)
	require.Equal(t, saga.StatusRunning, got.Status)
	require.Equal(t, int64(0), got.Version)

	got.Status = saga.StatusCompleted
	require.NoError(t, s.Update(ctx, got))
	require.Equal(t, int64(1), got.Version) // version bumped, written back

	reloaded, err := s.Get(ctx, "a")
	require.NoError(t, err)
	require.Equal(t, saga.StatusCompleted, reloaded.Status)
	require.Equal(t, int64(1), reloaded.Version)

	require.NoError(t, s.Delete(ctx, "a"))
	_, err = s.Get(ctx, "a")
	require.ErrorIs(t, err, sagaerrs.ErrInstanceNotFound)

	// Deleting a missing instance is not an error.
	require.NoError(t, s.Delete(ctx, "a"))
}

func TestIntegrationCreateDuplicate(t *testing.T) {
	t.Parallel()
	s := newIntegrationStore(t)
	ctx := t.Context()

	require.NoError(t, s.Create(ctx, itInstance("dup", saga.StatusRunning, time.Time{})))
	err := s.Create(ctx, itInstance("dup", saga.StatusRunning, time.Time{}))
	require.ErrorIs(t, err, sagaerrs.ErrInstanceExists)
}

func TestIntegrationVersionConflict(t *testing.T) {
	t.Parallel()
	s := newIntegrationStore(t)
	ctx := t.Context()

	require.NoError(t, s.Create(ctx, itInstance("v", saga.StatusRunning, time.Time{})))

	a, err := s.Get(ctx, "v")
	require.NoError(t, err)
	b := a.Clone()

	require.NoError(t, s.Update(ctx, a)) // a wins, version bumped
	err = s.Update(ctx, b)               // b carries the stale version
	require.ErrorIs(t, err, sagaerrs.ErrVersionConflict)
}

func TestIntegrationUpdateNotFound(t *testing.T) {
	t.Parallel()
	s := newIntegrationStore(t)
	ctx := t.Context()

	err := s.Update(ctx, itInstance("ghost", saga.StatusRunning, time.Time{}))
	require.ErrorIs(t, err, sagaerrs.ErrInstanceNotFound)
}

func TestIntegrationFetchRecoverable(t *testing.T) {
	t.Parallel()
	s := newIntegrationStore(t)
	ctx := t.Context()

	now := time.Now().UTC()
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	require.NoError(t, s.Create(ctx, itInstance("compensating", saga.StatusCompensating, time.Time{})))
	require.NoError(t, s.Create(ctx, itInstance("timed-out", saga.StatusRunning, past)))
	require.NoError(t, s.Create(ctx, itInstance("healthy", saga.StatusRunning, future)))
	require.NoError(t, s.Create(ctx, itInstance("done", saga.StatusCompleted, time.Time{})))

	got, err := s.FetchRecoverable(ctx, now, 0)
	require.NoError(t, err)

	ids := make(map[string]struct{}, len(got))
	for _, inst := range got {
		ids[inst.ID] = struct{}{}
	}
	require.Contains(t, ids, "compensating")
	require.Contains(t, ids, "timed-out")
	require.NotContains(t, ids, "healthy")
	require.NotContains(t, ids, "done")
}
