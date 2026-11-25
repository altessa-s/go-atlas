// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package retryqueue_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/reliability/retryqueue"
	"github.com/altessa-s/go-atlas/reliability/retryqueue/storages/memory"
)

// customStore exposes exactly the original operations. A named field keeps
// additional methods of the backing implementation outside its method set.
type customStore struct{ backend *memory.Store[string] }

var _ retryqueue.Storage[string] = customStore{}

func (s customStore) Mark(ctx context.Context, item retryqueue.Item[string]) (retryqueue.Item[string], error) {
	return s.backend.Mark(ctx, item)
}

func (s customStore) Reopen(ctx context.Context, item retryqueue.Item[string]) (retryqueue.Item[string], error) {
	return s.backend.Reopen(ctx, item)
}

func (s customStore) ListDue(ctx context.Context, now time.Time, limit int) ([]retryqueue.Item[string], error) {
	return s.backend.ListDue(ctx, now, limit)
}

func (s customStore) WakeDue(ctx context.Context, now time.Time) error {
	return s.backend.WakeDue(ctx, now)
}

func (s customStore) Reschedule(ctx context.Context, key string, generation int64, next time.Time) error {
	return s.backend.Reschedule(ctx, key, generation, next)
}

func (s customStore) Expire(ctx context.Context, key string, generation int64, since, at time.Time) (bool, error) {
	return s.backend.Expire(ctx, key, generation, since, at)
}

func (s customStore) Resolve(ctx context.Context, key string, generation int64, at time.Time) error {
	return s.backend.Resolve(ctx, key, generation, at)
}

func (s customStore) Flag(ctx context.Context, key string, generation int64, reason string) (bool, error) {
	return s.backend.Flag(ctx, key, generation, reason)
}

func (s customStore) ClearFlag(ctx context.Context, key string, generation int64) error {
	return s.backend.ClearFlag(ctx, key, generation)
}

func (s customStore) DeleteObsolete(ctx context.Context, key string, generation int64) (newer bool, err error) {
	return s.backend.DeleteObsolete(ctx, key, generation)
}

func (s customStore) Delete(ctx context.Context, key string) error {
	return s.backend.Delete(ctx, key)
}

func (s customStore) ListByState(ctx context.Context, state retryqueue.State, limit int) ([]retryqueue.Item[string], error) {
	return s.backend.ListByState(ctx, state, limit)
}

func (s customStore) Requeue(ctx context.Context, now time.Time, keys ...string) (int, error) {
	return s.backend.Requeue(ctx, now, keys...)
}

func TestCustomStorageCompatibility(t *testing.T) {
	t.Parallel()
	const originalMethods = 13
	require.Equal(t, originalMethods, reflect.TypeFor[retryqueue.Storage[string]]().NumMethod())
	var storage retryqueue.Storage[string] = customStore{memory.New[string]()}
	var producer retryqueue.ProducerStorage[string] = storage
	var admin retryqueue.AdminStorage[string] = storage
	item, err := producer.Mark(t.Context(), retryqueue.Item[string]{Key: "work", Payload: "value"})
	require.NoError(t, err)
	items, err := admin.ListByState(t.Context(), retryqueue.StatePending, 1)
	require.NoError(t, err)
	require.Equal(t, []retryqueue.Item[string]{item}, items)
	_, fenced := storage.(retryqueue.WorkerStorage[string])
	require.False(t, fenced)
}
