// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cas_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/data/mongo/cas"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

func TestInitialVersion(t *testing.T) {
	t.Parallel()

	before := time.Now().UnixNano()
	v := cas.InitialVersion()
	after := time.Now().UnixNano()
	require.GreaterOrEqual(t, v, before)
	require.LessOrEqual(t, v, after)
}

func TestFilter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		expected int64
	}{
		{name: "zero", expected: 0},
		{name: "one", expected: 1},
		{name: "positive", expected: 5},
		{name: "initial version", expected: 1_760_000_000_000_000_000},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, bson.M{"key": 7, "rev": tc.expected}, cas.Filter("key", 7, "rev", tc.expected))
		})
	}
}

func TestInc(t *testing.T) {
	t.Parallel()

	require.Equal(t, bson.M{"$inc": bson.M{"rev": int64(1)}}, cas.Inc("rev"))
}

// TestClassifyMissCountError: a failed check is returned as an error, never classified as NotFound or Conflict.
func TestClassifyMissCountError(t *testing.T) {
	t.Parallel()

	client, err := mongo.Connect(mongoOptions.Client().ApplyURI("mongodb://127.0.0.1:1").SetServerSelectionTimeout(time.Second))
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Disconnect(context.WithoutCancel(t.Context())) })

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	errNotFound, errConflict := errors.New("not found"), errors.New("conflict")
	err = cas.ClassifyMiss(ctx, client.Database("db").Collection("c"), "_id", "a", errNotFound, errConflict)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
	require.NotErrorIs(t, err, errNotFound)
	require.NotErrorIs(t, err, errConflict)
	require.NotErrorIs(t, err, cas.ErrNotFound)
	require.NotErrorIs(t, err, cas.ErrConflict)
}

// TestSentinelsWrapCanonical: the package sentinels keep their identity and messages and match the canonical core/errors ones.
func TestSentinelsWrapCanonical(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		err       error
		canonical error
		msg       string
	}{
		{name: "not found", err: cas.ErrNotFound, canonical: coreerrs.ErrNotFound, msg: "cas: document not found"},
		{name: "conflict", err: cas.ErrConflict, canonical: coreerrs.ErrConflict, msg: "cas: version conflict"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			wrapped := fmt.Errorf("op: %w", tc.err)
			require.ErrorIs(t, wrapped, tc.err)
			require.ErrorIs(t, wrapped, tc.canonical)
			require.EqualError(t, tc.err, tc.msg)
		})
	}
	require.NotErrorIs(t, cas.ErrNotFound, coreerrs.ErrConflict)
	require.NotErrorIs(t, cas.ErrConflict, coreerrs.ErrNotFound)
}
