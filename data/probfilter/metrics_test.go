// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package probfilter_test

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/probfilter"
	"github.com/altessa-s/go-atlas/data/probfilter/bloom"
	"github.com/altessa-s/go-atlas/data/probfilter/cuckoo"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	bloommemory "github.com/altessa-s/go-atlas/data/probfilter/bloom/storages/memory"
	bloomredis "github.com/altessa-s/go-atlas/data/probfilter/bloom/storages/redis"
	cuckoomemory "github.com/altessa-s/go-atlas/data/probfilter/cuckoo/storages/memory"
)

const (
	lookupsMetric         = "test_probfilter_lookups_total"
	addsMetric            = "test_probfilter_adds_total"
	lookupDurationMetric  = "test_probfilter_lookup_duration_seconds"
	rebuildDurationMetric = "test_probfilter_rebuild_duration_seconds"
	rebuildErrorsMetric   = "test_probfilter_rebuild_errors_total"
)

var errMetricsLoad = errors.New("load failed")

func TestManager_RecordsFilterMetrics(t *testing.T) {
	t.Parallel()

	filters := map[string]probfilter.RebuildableFilter{
		"bloom":  bloom.New(bloommemory.New(bloommemory.WithExpectedItems(100))),
		"cuckoo": cuckoo.New(cuckoomemory.New(cuckoomemory.WithCapacity(100))),
	}

	for name, f := range filters {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			require.NoError(t, f.Add(ctx, "a"))
			require.NoError(t, f.AddBatch(ctx, slices.Values([]string{"b", "c"})))
			// Fingerprints are seeded per filter: find a negative probe before
			// instrumenting, instead of assuming a value is no false positive.
			probe := ""
			for i := 0; probe == ""; i++ {
				v := fmt.Sprintf("probe-%d", i)
				if ok, err := f.MightExist(ctx, v); err == nil && !ok {
					probe = v
				}
			}

			tc := testhelpers.NewTestCollector()
			mgr := probfilter.NewManager(probfilter.WithCollector(tc))
			require.NoError(t, mgr.Register(name, f))

			require.NoError(t, f.Add(ctx, "a"))
			require.NoError(t, f.AddBatch(ctx, slices.Values([]string{"b", "c"})))
			ok, err := f.MightExist(ctx, "a")
			require.NoError(t, err)
			require.True(t, ok)
			ok, err = f.MightExist(ctx, probe)
			require.NoError(t, err)
			require.False(t, ok)

			require.NoError(t, f.Rebuild(ctx, probfilter.NewDataLoader(func() iter.Seq[string] {
				return slices.Values([]string{"x"})
			})))
			failing := probfilter.DataLoaderFunc(func(context.Context) iter.Seq2[string, error] {
				return func(yield func(string, error) bool) { yield("", errMetricsLoad) }
			})
			require.ErrorIs(t, f.Rebuild(ctx, failing), errMetricsLoad)

			require.Equal(t, 1.0, testhelpers.GetCounterValue(t, tc, lookupsMetric, "filter_name", name, "result", "positive"))
			require.Equal(t, 1.0, testhelpers.GetCounterValue(t, tc, lookupsMetric, "filter_name", name, "result", "negative"))
			require.Equal(t, 3.0, testhelpers.GetCounterValue(t, tc, addsMetric, "filter_name", name))
			require.Equal(t, uint64(2), testhelpers.GetHistogramCount(t, tc, lookupDurationMetric, "filter_name", name))
			require.Equal(t, uint64(2), testhelpers.GetHistogramCount(t, tc, rebuildDurationMetric))
			require.Equal(t, 1.0, testhelpers.GetCounterValue(t, tc, rebuildErrorsMetric))

			// Unregister detaches the observer: further operations are not recorded.
			mgr.Unregister(name)
			require.NoError(t, f.Add(ctx, "d"))
			require.Equal(t, 3.0, testhelpers.GetCounterValue(t, tc, addsMetric, "filter_name", name))
		})
	}
}

func TestManager_RecordsLookupErrors(t *testing.T) {
	t.Parallel()
	tc := testhelpers.NewTestCollector()
	mgr := probfilter.NewManager(probfilter.WithCollector(tc))
	f := bloom.New(&failingStorage{Storage: bloommemory.New()})
	require.NoError(t, mgr.Register("failing", f))

	_, err := f.MightExist(t.Context(), "a")
	require.ErrorIs(t, err, errMetricsLoad)
	require.Equal(t, 1.0, testhelpers.GetCounterValue(t, tc, lookupsMetric, "filter_name", "failing", "result", "error"))
}

// failingStorage is a Bloom storage whose lookups fail.
type failingStorage struct {
	*bloommemory.Storage
}

func (s *failingStorage) MightExist(context.Context, string) (bool, error) {
	return false, errMetricsLoad
}

func TestManager_NoCollectorAttachesNoObserver(t *testing.T) {
	t.Parallel()
	f := &observableMock{}
	mgr := probfilter.NewManager()
	require.NoError(t, mgr.Register("f", f))
	require.False(t, f.set, "without a collector the Manager must not instrument filters")

	tc := testhelpers.NewTestCollector()
	mgr = probfilter.NewManager(probfilter.WithCollector(tc))
	require.NoError(t, mgr.Register("f", f))
	require.NotNil(t, f.observer)
	require.NoError(t, mgr.Close())
	require.Nil(t, f.observer, "Close detaches the observer")
}

type observableMock struct {
	mockFilter
	set      bool
	observer probfilter.Observer
}

func (m *observableMock) SetObserver(o probfilter.Observer) {
	m.set = true
	m.observer = o
}

// TestManager_CloseWhileRebuildReadsManager closes the Manager while a
// filter's rebuild loader reads the Manager: Close must not hold the Manager
// lock while it waits for the rebuild.
func TestManager_CloseWhileRebuildReadsManager(t *testing.T) {
	t.Parallel()
	mgr := probfilter.NewManager()
	f := bloom.New(bloommemory.New())
	require.NoError(t, mgr.Register("f", f))

	started := make(chan struct{})
	loader := probfilter.DataLoaderFunc(func(ctx context.Context) iter.Seq2[string, error] {
		return func(yield func(string, error) bool) {
			close(started)
			<-ctx.Done() // canceled by Close
			_, _ = mgr.Get("f")
		}
	})
	rebuilt := make(chan error, 1)
	go func() { rebuilt <- f.Rebuild(t.Context(), loader) }()
	<-started

	closed := make(chan error, 1)
	go func() { closed <- mgr.Close() }()
	select {
	case err := <-closed:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Manager.Close deadlocked with a rebuild reading the Manager")
	}
	require.ErrorIs(t, <-rebuilt, probfilter.ErrFilterClosed)
}

// TestManager_RebuildSkipIsNoError reports rebuild outcomes through the
// Manager's observer: a rebuild skipped because another process rebuilds the
// shared filter is neither timed nor counted as failed; a superseded or
// otherwise failed rebuild is an error.
func TestManager_RebuildSkipIsNoError(t *testing.T) {
	t.Parallel()
	tc := testhelpers.NewTestCollector()
	mgr := probfilter.NewManager(probfilter.WithCollector(tc))
	f := &observableMock{}
	require.NoError(t, mgr.Register("shared", f))

	f.observer.ObserveRebuild(time.Millisecond, fmt.Errorf("acquire lease: %w", probfilter.ErrRebuildInProgress))
	require.Zero(t, testhelpers.GetCounterValue(t, tc, rebuildErrorsMetric))

	f.observer.ObserveRebuild(time.Millisecond, nil)
	f.observer.ObserveRebuild(time.Millisecond, fmt.Errorf("commit: %w", probfilter.ErrRebuildSuperseded))
	f.observer.ObserveRebuild(time.Millisecond, errMetricsLoad)
	require.Equal(t, 2.0, testhelpers.GetCounterValue(t, tc, rebuildErrorsMetric))
	require.Equal(t, uint64(3), testhelpers.GetHistogramCount(t, tc, rebuildDurationMetric))
}

// TestFacade_RefusedSharedRebuildRecordsNoError drives a real refusal: a peer
// holds the rebuild lease of a shared Redis filter (miniredis runs the lease
// scripts), so this process's Rebuild is refused and must not count as failed.
func TestFacade_RefusedSharedRebuildRecordsNoError(t *testing.T) {
	t.Parallel()
	client, _ := testhelpers.RedisClient(t)
	ctx := t.Context()

	started, release := make(chan struct{}), make(chan struct{})
	peer := bloom.New(bloomredis.New(client, "shared"))
	peerDone := make(chan struct{})
	go func() {
		defer close(peerDone)
		_ = peer.Rebuild(context.Background(), probfilter.DataLoaderFunc(func(context.Context) iter.Seq2[string, error] {
			return func(func(string, error) bool) {
				close(started)
				<-release
			}
		}))
	}()
	<-started
	t.Cleanup(func() {
		close(release)
		<-peerDone
	})

	tc := testhelpers.NewTestCollector()
	mgr := probfilter.NewManager(probfilter.WithCollector(tc))
	f := bloom.New(bloomredis.New(client, "shared"))
	require.NoError(t, mgr.Register("shared", f))

	require.ErrorIs(t, f.Rebuild(ctx, probfilter.NewDataLoader(func() iter.Seq[string] {
		return slices.Values([]string{"x"})
	})), probfilter.ErrRebuildInProgress)
	require.Zero(t, testhelpers.GetCounterValue(t, tc, rebuildErrorsMetric))
	require.Zero(t, testhelpers.GetHistogramCount(t, tc, rebuildDurationMetric))
}
