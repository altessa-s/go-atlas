// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongodb_test

import (
	"context"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/service/scheduler"
	"github.com/altessa-s/go-atlas/service/scheduler/storages/mongodb"

	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// mongoURI returns the MongoDB connection string, honoring MONGO_URI for CI.
func mongoURI() string {
	if uri := os.Getenv("MONGO_URI"); uri != "" {
		return uri
	}
	return "mongodb://localhost:27017"
}

// newClaimIT connects to a live MongoDB, skipping when none is reachable. Each
// call gets its own throwaway database (dropped on cleanup) and a Storage bound
// to it.
func newClaimIT(t *testing.T) *mongodb.Storage {
	t.Helper()

	client, err := mongo.Connect(mongoOptions.Client().ApplyURI(mongoURI()))
	if err != nil {
		t.Skipf("mongodb not available: %v", err)
	}
	if err := client.Ping(context.Background(), nil); err != nil {
		_ = client.Disconnect(context.Background())
		t.Skipf("mongodb not reachable at %s: %v", mongoURI(), err)
	}

	dbName := "sched_claim_it_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	db := client.Database(dbName)
	t.Cleanup(func() {
		_ = db.Drop(context.Background())
		_ = client.Disconnect(context.Background())
	})

	s := mongodb.New(db)

	// Capability probe: Ping succeeds without authentication, but the writes
	// these tests issue do not. Skip (rather than fail) when the server rejects
	// commands — e.g. an auth-required mongod with no credentials in the URI.
	probe := &scheduler.TaskState{TaskSummary: scheduler.TaskSummary{ID: "__probe__", Status: scheduler.TaskStatusActive}}
	if err := s.UpsertTask(context.Background(), probe); err != nil {
		t.Skipf("mongodb not usable for writes (need an unauthenticated server or credentials in MONGO_URI): %v", err)
	}
	if err := s.DeleteTask(context.Background(), "__probe__"); err != nil {
		t.Skipf("mongodb not usable for writes: %v", err)
	}

	return s
}

// seedActive inserts an active task due at nextRunAt.
func seedActive(t *testing.T, s *mongodb.Storage, id string, nextRunAt int64) {
	t.Helper()
	require.NoError(t, s.UpsertTask(t.Context(), &scheduler.TaskState{
		TaskSummary: scheduler.TaskSummary{ID: id, Status: scheduler.TaskStatusActive, NextRunAt: nextRunAt},
	}))
}

func TestIntegration_MongoClaimRun_SingleWinnerThenLost(t *testing.T) {
	t.Parallel()
	s := newClaimIT(t)
	ctx := t.Context()
	seedActive(t, s, "a", 100)

	ok, err := s.ClaimRun(ctx, "a", 100, 1700000000, "run-1")
	require.NoError(t, err)
	require.True(t, ok, "first claim must win")

	got, err := s.GetTask(ctx, "a")
	require.NoError(t, err)
	require.Equal(t, scheduler.TaskStatusRunning, got.Status)
	require.Equal(t, int64(1700000000), got.RunStartedAt)
	require.Equal(t, "run-1", got.LastRunID)

	// The occurrence is no longer active: a second claim loses.
	ok, err = s.ClaimRun(ctx, "a", 100, 1700000001, "run-2")
	require.NoError(t, err)
	require.False(t, ok, "second claim must lose")
}

func TestIntegration_MongoClaimRun_FenceMismatch(t *testing.T) {
	t.Parallel()
	s := newClaimIT(t)
	seedActive(t, s, "a", 100)

	ok, err := s.ClaimRun(t.Context(), "a", 999, 1700000000, "run-1")
	require.NoError(t, err)
	require.False(t, ok, "a mismatched occurrence fence must not be claimable")
}

func TestIntegration_MongoClaimRun_ZeroFenceIgnoresNextRun(t *testing.T) {
	t.Parallel()
	s := newClaimIT(t)

	ok, err := func() (bool, error) {
		seedActive(t, s, "a", 100)
		return s.ClaimRun(t.Context(), "a", 0, 1700000000, "run-1")
	}()
	require.NoError(t, err)
	require.True(t, ok, "expectedNextRunAt==0 claims any active occurrence")
}

func TestIntegration_MongoClaimRun_NotActive(t *testing.T) {
	t.Parallel()
	s := newClaimIT(t)
	require.NoError(t, s.UpsertTask(t.Context(), &scheduler.TaskState{
		TaskSummary: scheduler.TaskSummary{ID: "a", Status: scheduler.TaskStatusPaused, NextRunAt: 100},
	}))

	ok, err := s.ClaimRun(t.Context(), "a", 100, 1700000000, "run-1")
	require.NoError(t, err)
	require.False(t, ok, "a paused task must not be claimable")
}

func TestIntegration_MongoClaimRun_MissingTask(t *testing.T) {
	t.Parallel()
	s := newClaimIT(t)

	ok, err := s.ClaimRun(t.Context(), "nope", 100, 1700000000, "run-1")
	require.NoError(t, err)
	require.False(t, ok)
}

// TestIntegration_MongoClaimRun_ExactlyOneConcurrentWinner is the core
// anti-double-execution invariant verified against MongoDB's real atomic
// UpdateOne: many schedulers racing on one occurrence yield exactly one winner.
func TestIntegration_MongoClaimRun_ExactlyOneConcurrentWinner(t *testing.T) {
	t.Parallel()
	s := newClaimIT(t)
	ctx := t.Context()
	seedActive(t, s, "a", 100)

	const racers = 32
	var wins atomic.Int64
	var wg sync.WaitGroup
	start := make(chan struct{})
	for range racers {
		wg.Go(func() {
			<-start
			ok, err := s.ClaimRun(ctx, "a", 100, 1700000000, "run")
			require.NoError(t, err)
			if ok {
				wins.Add(1)
			}
		})
	}
	close(start)
	wg.Wait()

	require.Equal(t, int64(1), wins.Load(), "exactly one claim may win")
}
