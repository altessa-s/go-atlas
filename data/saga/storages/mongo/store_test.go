// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas/data/saga"
)

func TestDefaultOptions(t *testing.T) {
	t.Parallel()
	opts := defaultOptions()
	require.Equal(t, DefaultCollectionName, opts.collectionName)
	require.Equal(t, DefaultIndexCreateTimeout, opts.indexTimeout)
	require.Nil(t, opts.ctx)
}

func TestWithCollectionName(t *testing.T) {
	t.Parallel()
	require.Equal(t, "custom", newOptions(WithCollectionName("custom")).collectionName)
	require.Equal(t, "trimmed", newOptions(WithCollectionName("  trimmed  ")).collectionName)
	require.Equal(t, DefaultCollectionName, newOptions(WithCollectionName("")).collectionName)
}

func TestWithIndexCreateTimeout(t *testing.T) {
	t.Parallel()
	require.Equal(t, 30*time.Second, newOptions(WithIndexCreateTimeout(30*time.Second)).indexTimeout)
	require.Equal(t, DefaultIndexCreateTimeout, newOptions(WithIndexCreateTimeout(0)).indexTimeout)
	require.Equal(t, DefaultIndexCreateTimeout, newOptions(WithIndexCreateTimeout(-time.Second)).indexTimeout)
}

func TestWithContext(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	require.Equal(t, ctx, newOptions(WithContext(ctx)).ctx)
}

func TestUnixRoundTripZero(t *testing.T) {
	t.Parallel()
	require.Equal(t, int64(0), toUnix(time.Time{}))
	require.True(t, fromUnix(0).IsZero())

	now := time.Now().UTC().Truncate(time.Second)
	require.Equal(t, now, fromUnix(toUnix(now)))
}

func TestDocumentRoundTrip(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	inst := &saga.Instance{
		ID:         "order-1",
		Definition: "place-order",
		Status:     saga.StatusCompensating,
		Stage:      2,
		Data:       []byte(`{"n":1}`),
		Steps: []saga.StepRecord{
			{Name: "reserve", Stage: 0, Status: saga.StepCompleted, Attempts: 1, StartedAt: now, FinishedAt: now},
			{Name: "charge", Stage: 1, Status: saga.StepFailed, Attempts: 3, Error: "declined"},
		},
		CreatedAt: now,
		UpdatedAt: now,
		Deadline:  now.Add(time.Minute),
		Version:   7,
		LastError: "boom",
	}

	doc := toDocument(inst)
	require.Equal(t, inst, fromDocument(&doc))
}

func TestDocumentZeroDeadlineMapsToUnset(t *testing.T) {
	t.Parallel()
	inst := &saga.Instance{ID: "x", Status: saga.StatusRunning}
	doc := toDocument(inst)
	require.Equal(t, int64(0), doc.Deadline)
	require.True(t, fromDocument(&doc).Deadline.IsZero())
}

func TestInstanceIndexesCoverStatusDeadline(t *testing.T) {
	t.Parallel()
	require.Len(t, instanceIndexes, 1)

	keys, ok := instanceIndexes[0].Keys.(bson.D)
	require.True(t, ok)

	got := make([]string, len(keys))
	for i, e := range keys {
		got[i] = e.Key
	}
	require.Equal(t, []string{collectionFieldStatus, collectionFieldDeadline}, got)
}
