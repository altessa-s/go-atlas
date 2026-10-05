// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package secrets_test

import (
	"context"
	"iter"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/security/secrets"

	secretmemory "github.com/altessa-s/go-atlas/security/secrets/providers/memory"
)

// payloadOf returns a printable form of a string or []byte payload.
func payloadOf[T string | []byte](v T) string { return string(v) }

// testProviderMemoryIntact caches keys through every Manager ingest path,
// then clears through every Manager clear path, and checks that the memory
// provider's own values are untouched.
func testProviderMemoryIntact[T string | []byte](t *testing.T, mk func(string) T) {
	t.Helper()
	ctx := t.Context()

	inner, err := secretmemory.New(map[string]T{"key-a": mk("value-a"), "key-b": mk("value-b"), "key-c": mk("value-c")})
	require.NoError(t, err)
	mgr, err := secrets.New[T](inner)
	require.NoError(t, err)

	_, err = mgr.Value(ctx, "key-a", true)
	require.NoError(t, err)
	require.NoError(t, mgr.Save(ctx, "key-d", mk("value-d")))
	require.NoError(t, mgr.RunUpdateCycle(ctx))

	require.NoError(t, mgr.Delete(ctx, "key-c"))
	mgr.ClearCache(ctx)
	require.NoError(t, mgr.RunUpdateCycle(ctx))
	mgr.Shutdown()

	for key, want := range map[string]string{"key-a": "value-a", "key-b": "value-b", "key-d": "value-d"} {
		v, err := inner.Value(ctx, key)
		require.NoError(t, err)
		require.Equal(t, want, payloadOf(v.Value), "the Manager must not clear provider-owned memory (%s)", key)
	}
}

// TestManager_ClearDoesNotTouchProviderMemory checks that the Manager only
// ever clears its own copies.
func TestManager_ClearDoesNotTouchProviderMemory(t *testing.T) {
	t.Parallel()
	t.Run("string", func(t *testing.T) {
		t.Parallel()
		testProviderMemoryIntact(t, func(s string) string { return string([]byte(s)) })
	})
	t.Run("bytes", func(t *testing.T) {
		t.Parallel()
		testProviderMemoryIntact(t, func(s string) []byte { return []byte(s) })
	})
}

// testSaveDoesNotShare saves a cached payload under another key and clears
// the source: the provider's copy of the destination must survive.
func testSaveDoesNotShare[T string | []byte](t *testing.T, mk func(string) T) {
	t.Helper()
	ctx := t.Context()

	inner, err := secretmemory.New(map[string]T{"src-key": mk("payload")})
	require.NoError(t, err)
	mgr, err := secrets.New[T](inner)
	require.NoError(t, err)

	src, err := mgr.Value(ctx, "src-key", true)
	require.NoError(t, err)
	require.NoError(t, mgr.Save(ctx, "dst-key", src.Value))
	require.NoError(t, mgr.Delete(ctx, "src-key"))
	mgr.ClearCache(ctx)

	dst, err := inner.Value(ctx, "dst-key")
	require.NoError(t, err)
	require.Equal(t, "payload", payloadOf(dst.Value))
}

// TestManager_SaveDoesNotShareCallerPayload checks that Save hands the
// provider its own copy of the payload.
func TestManager_SaveDoesNotShareCallerPayload(t *testing.T) {
	t.Parallel()
	t.Run("string", func(t *testing.T) {
		t.Parallel()
		testSaveDoesNotShare(t, func(s string) string { return string([]byte(s)) })
	})
	t.Run("bytes", func(t *testing.T) {
		t.Parallel()
		testSaveDoesNotShare(t, func(s string) []byte { return []byte(s) })
	})
}

// TestManager_ValueForceReturnsManagerOwnedCopy checks that a fetched value
// is the Manager's copy, equal to but distinct from the provider's instance.
func TestManager_ValueForceReturnsManagerOwnedCopy(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	inner, err := secretmemory.New(map[string]string{"key-a": "value-a"})
	require.NoError(t, err)
	mgr, err := secrets.New[string](inner)
	require.NoError(t, err)

	got, err := mgr.Value(ctx, "key-a", true)
	require.NoError(t, err)
	orig, err := inner.Value(ctx, "key-a")
	require.NoError(t, err)
	require.NotSame(t, orig, got)
	require.Equal(t, orig.Key, got.Key)
	require.Equal(t, orig.Version, got.Version)
	require.Equal(t, orig.Value, got.Value)

	again, err := mgr.Value(ctx, "key-a", false)
	require.NoError(t, err)
	require.NotSame(t, got, again, "every Value call returns its own copy")
	require.Equal(t, got.Value, again.Value)
}

// secretString is a defined string payload type.
type secretString string

// literalProvider returns Value literals of a defined string type, as a
// remote provider may build them.
type literalProvider struct{}

func (literalProvider) Name() string { return "literal" }

func (literalProvider) List(context.Context) ([]*secrets.Value[secretString], error) {
	return []*secrets.Value[secretString]{{Key: "key-a", Value: "value-a", EncodedValue: []byte("value-a"), Version: "1"}}, nil
}

func (literalProvider) Values(context.Context) iter.Seq2[*secrets.Value[secretString], error] {
	return func(func(*secrets.Value[secretString], error) bool) {}
}

func (literalProvider) Value(_ context.Context, key string) (*secrets.Value[secretString], error) {
	return &secrets.Value[secretString]{Key: key, Value: "value-a", EncodedValue: []byte("value-a"), Version: "1"}, nil
}

func (literalProvider) Save(context.Context, string, secretString) error { return nil }
func (literalProvider) Delete(context.Context, string) error             { return nil }

// TestManager_DefinedStringPayload checks that values of a defined string
// type are fetched, cycled and cleared without panicking.
func TestManager_DefinedStringPayload(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	mgr, err := secrets.New[secretString](literalProvider{})
	require.NoError(t, err)

	got, err := mgr.Value(ctx, "key-a", true)
	require.NoError(t, err)
	require.Equal(t, secretString("value-a"), got.Value)
	require.NoError(t, mgr.RunUpdateCycle(ctx))
	require.NoError(t, mgr.Save(ctx, "key-b", "value-b"))
	mgr.ClearCache(ctx)
}

// TestManager_Watch_EventValuesAreReceiverOwned checks that a receiver may
// clear its event values without affecting the cache or later events.
func TestManager_Watch_EventValuesAreReceiverOwned(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	// key-b keeps the listing non-empty after key-a is deleted.
	provider := &versionedProvider{values: map[string]string{"key-a": "value-a", "key-b": "value-b"}}
	mgr, err := secrets.New[string](provider)
	require.NoError(t, err)
	watch, err := mgr.Watch(ctx, secrets.WatchOptions{Keys: []string{"key-a"}})
	require.NoError(t, err)
	t.Cleanup(watch.Stop)

	require.NoError(t, mgr.RunUpdateCycle(ctx))
	ev := <-watch.Events
	require.Equal(t, secrets.EventTypeCreated, ev.Type)
	require.Equal(t, "value-a", ev.Value.Value)
	ev.Value.Clear()

	cached, err := mgr.Value(ctx, "key-a", false)
	require.NoError(t, err)
	require.Equal(t, "value-a", cached.Value)
	require.NotSame(t, cached, ev.Value)

	require.NoError(t, mgr.RunUpdateCycle(ctx))
	select {
	case ev := <-watch.Events:
		t.Fatalf("unexpected event %s for %q after the receiver cleared its copy", ev.Type, ev.Key)
	default:
	}

	require.NoError(t, provider.Delete(ctx, "key-a"))
	require.NoError(t, mgr.RunUpdateCycle(ctx))
	ev = <-watch.Events
	require.Equal(t, secrets.EventTypeDeleted, ev.Type)
	require.Equal(t, "value-a", ev.PreviousValue.Value)
}

// TestManager_Watch_ConcurrentClearDuringNotify reads every event payload
// while Delete and ClearCache clear cached values concurrently with update
// cycles. Run with -race.
func TestManager_Watch_ConcurrentClearDuringNotify(t *testing.T) {
	t.Parallel()

	providers := map[string]func() secrets.Provider[string]{
		"memory": func() secrets.Provider[string] {
			p, err := secretmemory.New(map[string]string{"key-0": "v", "key-1": "v", "key-2": "v"})
			require.NoError(t, err)
			return p
		},
		"versioned": func() secrets.Provider[string] {
			return &versionedProvider{values: map[string]string{"key-0": "v", "key-1": "v", "key-2": "v"}}
		},
	}
	for name, newProvider := range providers {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()

			mgr, err := secrets.New[string](newProvider(), secrets.WithMaxRetries(1))
			require.NoError(t, err)
			watch, err := mgr.Watch(ctx, secrets.WatchOptions{BufferSize: 1000})
			require.NoError(t, err)

			var readers sync.WaitGroup
			readers.Go(func() {
				for ev := range watch.Events {
					for _, v := range []*secrets.Value[string]{ev.Value, ev.PreviousValue} {
						if v != nil {
							_, _, _ = v.Key, v.Version, v.Value
							v.Clear()
						}
					}
				}
			})

			const iterations = 200
			keys := []string{"key-0", "key-1", "key-2"}
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
					mgr.ClearCache(ctx)
				}
			})
			wg.Wait()
			watch.Stop()
			readers.Wait()
		})
	}
}

// TestManager_UpdateCycle_EmptyListingKeepsCache pins the documented
// behavior: a successful empty listing does not reconcile the cache.
func TestManager_UpdateCycle_EmptyListingKeepsCache(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	provider := &versionedProvider{values: map[string]string{"key-a": "value-a"}}
	mgr, err := secrets.New[string](provider)
	require.NoError(t, err)
	require.NoError(t, mgr.RunUpdateCycle(ctx))

	require.NoError(t, provider.Delete(ctx, "key-a"))
	require.NoError(t, mgr.RunUpdateCycle(ctx))

	got, err := mgr.Value(ctx, "key-a", false)
	require.NoError(t, err)
	require.Equal(t, "value-a", got.Value)
}

// TestManager_Watch_UpdatedEventCarriesPreviousValue checks that an Updated
// event carries both payloads as receiver-owned copies: clearing them leaves
// the cache and another watcher's events intact.
func TestManager_Watch_UpdatedEventCarriesPreviousValue(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	provider := &versionedProvider{values: map[string]string{"key-a": "v1"}}
	mgr, err := secrets.New[string](provider)
	require.NoError(t, err)
	first, err := mgr.Watch(ctx, secrets.WatchOptions{SkipInitialEvents: true})
	require.NoError(t, err)
	t.Cleanup(first.Stop)
	second, err := mgr.Watch(ctx, secrets.WatchOptions{SkipInitialEvents: true})
	require.NoError(t, err)
	t.Cleanup(second.Stop)

	require.NoError(t, mgr.RunUpdateCycle(ctx))
	require.NoError(t, provider.Save(ctx, "key-a", "v2"))
	require.NoError(t, mgr.RunUpdateCycle(ctx))

	ev := <-first.Events
	require.Equal(t, secrets.EventTypeUpdated, ev.Type)
	require.Equal(t, "v2", ev.Value.Value)
	require.NotNil(t, ev.PreviousValue)
	require.Equal(t, "v1", ev.PreviousValue.Value)
	ev.Value.Clear()
	ev.PreviousValue.Clear()

	other := <-second.Events
	require.Equal(t, "v2", other.Value.Value)
	require.Equal(t, "v1", other.PreviousValue.Value)
	require.NotSame(t, ev.Value, other.Value)

	cached, err := mgr.Value(ctx, "key-a", false)
	require.NoError(t, err)
	require.Equal(t, "v2", cached.Value)
}
