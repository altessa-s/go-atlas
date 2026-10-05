// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package secrets_test

import (
	"bytes"
	"context"
	"iter"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/probfilter"
	"github.com/altessa-s/go-atlas/data/probfilter/cuckoo"
	"github.com/altessa-s/go-atlas/security/secrets"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	cuckoomemory "github.com/altessa-s/go-atlas/data/probfilter/cuckoo/storages/memory"
	secretmemory "github.com/altessa-s/go-atlas/security/secrets/providers/memory"
)

// versionedProvider is an in-memory provider whose values carry a version
// derived from their content, so the update cycle can tell an old value from
// a newer one. List returns fresh Value instances, like a remote provider.
type versionedProvider struct {
	mu     sync.Mutex
	values map[string]string
}

func (p *versionedProvider) Name() string { return "versioned" }

func (p *versionedProvider) newValue(key, value string) *secrets.Value[string] {
	return secrets.NewValue(key, value, []byte(value), "v-"+value)
}

func (p *versionedProvider) List(_ context.Context) ([]*secrets.Value[string], error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	list := make([]*secrets.Value[string], 0, len(p.values))
	for k, v := range p.values {
		list = append(list, p.newValue(k, v))
	}
	return list, nil
}

func (p *versionedProvider) Values(ctx context.Context) iter.Seq2[*secrets.Value[string], error] {
	return func(yield func(*secrets.Value[string], error) bool) {
		list, err := p.List(ctx)
		if err != nil {
			yield(nil, err)
			return
		}
		for _, v := range list {
			if !yield(v, nil) {
				return
			}
		}
	}
}

func (p *versionedProvider) Value(_ context.Context, key string) (*secrets.Value[string], error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	v, ok := p.values[key]
	if !ok {
		return nil, secrets.ErrNotFound
	}
	return p.newValue(key, v), nil
}

func (p *versionedProvider) Save(_ context.Context, key, value string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.values[key] = value
	return nil
}

func (p *versionedProvider) Delete(_ context.Context, key string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.values, key)
	return nil
}

// TestManager_UpdateCycle_KeepsValueSavedAfterSnapshot saves a new key after
// the update cycle listed the storage: the cycle must neither evict nor clear
// the cached value, although the key is missing from its snapshot.
func TestManager_UpdateCycle_KeepsValueSavedAfterSnapshot(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		filter func() secrets.Filter
	}{
		{name: "no negative filter", filter: func() secrets.Filter { return nil }},
		{name: "rebuildable negative filter", filter: func() secrets.Filter {
			return cuckoo.New(cuckoomemory.New(cuckoomemory.WithCapacity(100)))
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()

			inner, err := secretmemory.New(map[string]string{"existing-key": "v"})
			require.NoError(t, err)
			provider := &listHookProvider{Provider: inner}

			opts := []secrets.Option{}
			if f := tc.filter(); f != nil {
				opts = append(opts, secrets.WithNegativeFilter(f))
			}
			mgr, err := secrets.New[string](provider, opts...)
			require.NoError(t, err)

			var saved *secrets.Value[string]
			provider.afterList = func() {
				require.NoError(t, mgr.Save(ctx, "saved-during-cycle", "new"))
				v, valueErr := mgr.Value(ctx, "saved-during-cycle", false)
				require.NoError(t, valueErr)
				saved = v
			}
			require.NoError(t, mgr.RunUpdateCycle(ctx))

			got, err := mgr.Value(ctx, "saved-during-cycle", false)
			require.NoError(t, err, "a value saved after the snapshot must stay cached")
			require.Equal(t, "new", got.Value)
			require.Equal(t, "new", saved.Value, "a value saved after the snapshot must not be cleared")
		})
	}
}

// TestManager_UpdateCycle_DoesNotOverwriteValueSavedAfterSnapshot saves a
// newer version of a listed key after the snapshot: the cycle must not put
// the older snapshot version back over it.
func TestManager_UpdateCycle_DoesNotOverwriteValueSavedAfterSnapshot(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	inner := &versionedProvider{values: map[string]string{"k": "old"}}
	provider := &listHookProvider{Provider: inner}
	mgr, err := secrets.New[string](provider)
	require.NoError(t, err)

	_, err = mgr.Value(ctx, "k", true)
	require.NoError(t, err)

	provider.afterList = func() {
		require.NoError(t, mgr.Save(ctx, "k", "new"))
	}
	require.NoError(t, mgr.RunUpdateCycle(ctx))

	got, err := mgr.Value(ctx, "k", false)
	require.NoError(t, err)
	require.Equal(t, "new", got.Value, "the cycle must not overwrite a value saved after its snapshot")

	// The next cycle sees the saved version and keeps it.
	require.NoError(t, mgr.RunUpdateCycle(ctx))
	got, err = mgr.Value(ctx, "k", false)
	require.NoError(t, err)
	require.Equal(t, "new", got.Value)
}

// TestManager_UpdateCycle_StillEvictsDeletedKeys checks that a key deleted
// from storage before the snapshot is still evicted and cleared.
func TestManager_UpdateCycle_StillEvictsDeletedKeys(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	inner := &versionedProvider{values: map[string]string{"gone": "x", "kept": "y"}}
	mgr, err := secrets.New[string](inner)
	require.NoError(t, err)
	require.NoError(t, mgr.RunUpdateCycle(ctx))

	cached, err := mgr.Value(ctx, "gone", false)
	require.NoError(t, err)

	require.NoError(t, inner.Delete(ctx, "gone"))
	require.NoError(t, mgr.RunUpdateCycle(ctx))

	_, err = mgr.Value(ctx, "gone", false)
	require.ErrorIs(t, err, secrets.ErrNotFound)
	require.Empty(t, cached.Value, "an evicted value is cleared")
	_, err = mgr.Value(ctx, "kept", false)
	require.NoError(t, err)
}

// stubRebuildFilter is a rebuildable negative filter whose Rebuild returns a
// fixed error, optionally after running the loader.
type stubRebuildFilter struct {
	secrets.Filter
	runLoader bool
	err       error
}

func (f *stubRebuildFilter) Rebuild(ctx context.Context, loader probfilter.DataLoader) error {
	if f.runLoader {
		for _, err := range loader.StreamValues(ctx) {
			if err != nil {
				return err
			}
		}
	}
	return f.err
}

func (f *stubRebuildFilter) LastRebuild() time.Time { return time.Time{} }

// TestManager_UpdateCycle_RebuildOutcomeLogging checks how the cycle reports
// negative-filter rebuild failures: a rebuild skipped because another process
// rebuilds the shared filter is benign (debug), a superseded rebuild is an
// error. Neither fails the cycle.
func TestManager_UpdateCycle_RebuildOutcomeLogging(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		runLoader bool
		err       error
		wantError bool
	}{
		{name: "in progress elsewhere", err: coreerrs.WrapOperation(probfilter.ErrRebuildInProgress, "acquire lease")},
		{name: "superseded", runLoader: true, err: coreerrs.WrapOperation(probfilter.ErrRebuildSuperseded, "commit"), wantError: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()

			var logs bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
			provider := &versionedProvider{values: map[string]string{"k": "v"}}
			filter := &stubRebuildFilter{
				Filter:    cuckoo.New(cuckoomemory.New(cuckoomemory.WithCapacity(100))),
				runLoader: tc.runLoader,
				err:       tc.err,
			}
			mgr, err := secrets.New[string](provider, secrets.WithNegativeFilter(filter), secrets.WithLogger(logger))
			require.NoError(t, err)

			require.NoError(t, mgr.RunUpdateCycle(ctx))
			_, err = mgr.Value(ctx, "k", false)
			require.NoError(t, err, "the cycle still refreshes the cache")

			if tc.wantError {
				require.Contains(t, logs.String(), "level=ERROR")
				require.Contains(t, logs.String(), "failed to rebuild negative filter")
				return
			}
			require.NotContains(t, logs.String(), "level=ERROR")
			require.Contains(t, logs.String(), "negative filter rebuild skipped")
		})
	}
}

// TestManager_UpdateCycle_DoesNotResurrectKeyDeletedAfterSnapshot deletes a
// cached key after the cycle listed it: the cycle must not put the listed,
// now deleted value back into the cache.
func TestManager_UpdateCycle_DoesNotResurrectKeyDeletedAfterSnapshot(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	inner := &versionedProvider{values: map[string]string{"k": "v", "other": "o"}}
	provider := &listHookProvider{Provider: inner}
	mgr, err := secrets.New[string](provider)
	require.NoError(t, err)

	provider.afterList = func() {
		require.NoError(t, mgr.Delete(ctx, "k"))
	}
	require.NoError(t, mgr.RunUpdateCycle(ctx))

	_, err = mgr.Value(ctx, "k", false)
	require.ErrorIs(t, err, secrets.ErrNotFound, "a key deleted after the snapshot must not be re-inserted")
	_, err = mgr.Value(ctx, "other", false)
	require.NoError(t, err)

	require.NoError(t, mgr.RunUpdateCycle(ctx))
	_, err = mgr.Value(ctx, "k", false)
	require.ErrorIs(t, err, secrets.ErrNotFound)
}

// keySpyFilter is a rebuildable filter that records the keys its rebuild
// loader yields.
type keySpyFilter struct {
	*cuckoo.Filter
	mu   sync.Mutex
	keys []string
}

func (f *keySpyFilter) Rebuild(ctx context.Context, loader probfilter.DataLoader) error {
	return f.Filter.Rebuild(ctx, probfilter.DataLoaderFunc(func(ctx context.Context) iter.Seq2[string, error] {
		return func(yield func(string, error) bool) {
			for k, err := range loader.StreamValues(ctx) {
				if err == nil {
					f.mu.Lock()
					f.keys = append(f.keys, k)
					f.mu.Unlock()
				}
				if !yield(k, err) {
					return
				}
			}
		}
	}))
}

// TestManager_UpdateCycle_DeleteAfterSnapshotMemoryProvider deletes a cached
// key after the listing with the memory provider, which returns the very
// instance the cache holds, so the Delete clears the listed value too. The
// cycle must neither cache it (under its key or an empty one), nor hand an
// empty key to the filter rebuild, nor report it to watchers.
func TestManager_UpdateCycle_DeleteAfterSnapshotMemoryProvider(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	inner, err := secretmemory.New(map[string]string{"key-a": "1", "key-b": "2"})
	require.NoError(t, err)
	provider := &listHookProvider{Provider: inner}
	filter := &keySpyFilter{Filter: cuckoo.New(cuckoomemory.New(cuckoomemory.WithCapacity(100)))}
	mgr, err := secrets.New[string](provider, secrets.WithNegativeFilter(filter))
	require.NoError(t, err)
	require.NoError(t, filter.Add(ctx, "key-a"))
	require.NoError(t, filter.Add(ctx, "key-b"))

	_, err = mgr.Value(ctx, "key-a", true)
	require.NoError(t, err)
	_, err = mgr.Value(ctx, "key-b", true)
	require.NoError(t, err)

	watch, err := mgr.Watch(ctx, secrets.WatchOptions{})
	require.NoError(t, err)
	t.Cleanup(watch.Stop)

	provider.afterList = func() {
		require.NoError(t, mgr.Delete(ctx, "key-a"))
	}
	require.NoError(t, mgr.RunUpdateCycle(ctx))

	_, err = mgr.Value(ctx, "key-a", false)
	require.ErrorIs(t, err, secrets.ErrNotFound)
	_, err = mgr.Value(ctx, "", false)
	require.ErrorIs(t, err, secrets.ErrNotFound)
	b, err := mgr.Value(ctx, "key-b", false)
	require.NoError(t, err)
	require.Equal(t, "2", b.Value)
	require.Equal(t, 1, mgr.CacheSize())

	filter.mu.Lock()
	require.Equal(t, []string{"key-b"}, filter.keys, "the rebuild must only see captured, uncleared keys")
	filter.mu.Unlock()

	for {
		select {
		case ev := <-watch.Events:
			require.NotEmpty(t, ev.Key, "no event for a cleared value")
			continue
		default:
		}
		break
	}
}

// TestManager_UpdateCycle_ConcurrentDeleteClearCache runs update cycles
// against concurrent Save, Delete and ClearCache calls over the memory
// provider, which shares value instances with the cache. Run with -race.
func TestManager_UpdateCycle_ConcurrentDeleteClearCache(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	keys := []string{"k0", "k1", "k2", "k3"}
	inner, err := secretmemory.New(map[string]string{"k0": "v", "k1": "v", "k2": "v", "k3": "v"})
	require.NoError(t, err)
	filter := cuckoo.New(cuckoomemory.New(cuckoomemory.WithCapacity(100)))
	mgr, err := secrets.New[string](inner, secrets.WithNegativeFilter(filter), secrets.WithMaxRetries(1))
	require.NoError(t, err)

	const iterations = 200
	var wg sync.WaitGroup
	wg.Go(func() {
		for range iterations {
			_ = mgr.RunUpdateCycle(ctx)
		}
	})
	wg.Go(func() {
		for i := range iterations {
			key := keys[i%len(keys)]
			_ = mgr.Delete(ctx, key)
			_ = mgr.Save(ctx, key, "v")
		}
	})
	wg.Go(func() {
		for range iterations {
			mgr.ClearCache(ctx)
		}
	})
	wg.Wait()

	// ClearCache may have zeroed values the memory provider still holds (it
	// shares them with the cache), and a racing Save may have cached such a
	// cleared value; reset both sides before checking that a final cycle
	// caches every key.
	mgr.ClearCache(ctx)
	for _, key := range keys {
		require.NoError(t, inner.Save(ctx, key, "v"))
	}
	require.NoError(t, mgr.RunUpdateCycle(ctx))
	for _, key := range keys {
		v, err := mgr.Value(ctx, key, false)
		require.NoError(t, err)
		require.Equal(t, key, v.Key)
	}
}

// TestManager_UpdateCycle_SharedProviderTwoManagers runs two managers over
// one memory provider concurrently; the cycle's bookkeeping is per manager,
// so nothing is written into the shared value instances. Run with -race.
func TestManager_UpdateCycle_SharedProviderTwoManagers(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	inner, err := secretmemory.New(map[string]string{"key-a": "1", "key-b": "2"})
	require.NoError(t, err)
	mgrA, err := secrets.New[string](inner)
	require.NoError(t, err)
	mgrB, err := secrets.New[string](inner)
	require.NoError(t, err)

	const iterations = 200
	var wg sync.WaitGroup
	for _, mgr := range []*secrets.Manager[string]{mgrA, mgrB} {
		wg.Go(func() {
			for range iterations {
				_ = mgr.RunUpdateCycle(ctx)
			}
		})
		wg.Go(func() {
			for i := range iterations {
				_ = mgr.Save(ctx, "key-a", "1")
				_, _ = mgr.Value(ctx, "key-b", i%2 == 0)
			}
		})
	}
	wg.Wait()

	for _, mgr := range []*secrets.Manager[string]{mgrA, mgrB} {
		require.NoError(t, mgr.RunUpdateCycle(ctx))
		v, err := mgr.Value(ctx, "key-a", false)
		require.NoError(t, err)
		require.Equal(t, "1", v.Value)
	}
}
