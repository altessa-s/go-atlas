// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package audit_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/audit"
	"github.com/altessa-s/go-atlas/data/audit/storages/memory"
)

func TestDispatcher_BatchFlush(t *testing.T) {
	store := memory.New()
	a, err := audit.New(store,
		audit.WithBatchSize(5),
		audit.WithFlushInterval(50*time.Millisecond),
		audit.WithWorkers(1),
	)
	require.NoError(t, err)
	require.NoError(t, a.Start())

	for range 5 {
		a.Emit(&audit.Event{Type: audit.EventTypeSystem, Action: audit.ActionExecute})
	}

	time.Sleep(200 * time.Millisecond)
	assert.Equal(t, 5, store.Len())

	require.NoError(t, a.Shutdown(t.Context()))
}

func TestDispatcher_TimerFlush(t *testing.T) {
	store := memory.New()
	a, err := audit.New(store,
		audit.WithBatchSize(100),
		audit.WithFlushInterval(50*time.Millisecond),
		audit.WithWorkers(1),
	)
	require.NoError(t, err)
	require.NoError(t, a.Start())

	for range 3 {
		a.Emit(&audit.Event{Type: audit.EventTypeSystem, Action: audit.ActionExecute})
	}

	time.Sleep(200 * time.Millisecond)
	assert.Equal(t, 3, store.Len())

	require.NoError(t, a.Shutdown(t.Context()))
}

func TestDispatcher_GracefulShutdown(t *testing.T) {
	store := memory.New()
	a, err := audit.New(store,
		audit.WithBatchSize(1000),
		audit.WithFlushInterval(time.Hour),
		audit.WithWorkers(1),
	)
	require.NoError(t, err)
	require.NoError(t, a.Start())

	for range 25 {
		a.Emit(&audit.Event{Type: audit.EventTypeSystem, Action: audit.ActionExecute})
	}

	require.NoError(t, a.Shutdown(t.Context()))
	assert.Equal(t, 25, store.Len())
}

func TestDispatcher_RetryOnFailure(t *testing.T) {
	var attempts atomic.Int32
	store := &failingStorage{
		Storage:   memory.New(),
		failCount: 2,
		attempts:  &attempts,
	}

	a, err := audit.New(store,
		audit.WithBatchSize(1),
		audit.WithFlushInterval(50*time.Millisecond),
		audit.WithWorkers(1),
		audit.WithRetryAttempts(3),
		audit.WithRetryBackoff(10*time.Millisecond),
	)
	require.NoError(t, err)
	require.NoError(t, a.Start())

	a.Emit(&audit.Event{Type: audit.EventTypeSystem, Action: audit.ActionExecute})

	time.Sleep(500 * time.Millisecond)
	require.NoError(t, a.Shutdown(t.Context()))

	assert.GreaterOrEqual(t, int(attempts.Load()), 3)
	assert.Equal(t, 1, store.Storage.Len())
}

// failingStorage wraps memory.Storage and fails the first N calls.
type failingStorage struct {
	*memory.Storage
	failCount int
	attempts  *atomic.Int32
}

func (f *failingStorage) Store(ctx context.Context, event *audit.Event) error {
	n := int(f.attempts.Add(1))
	if n <= f.failCount {
		return errors.New("simulated storage failure")
	}
	return f.Storage.Store(ctx, event)
}

func (f *failingStorage) StoreBatch(ctx context.Context, events []*audit.Event) error {
	n := int(f.attempts.Add(1))
	if n <= f.failCount {
		return errors.New("simulated storage failure")
	}
	return f.Storage.StoreBatch(ctx, events)
}
