// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package secrets_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/security/secrets"
	"github.com/altessa-s/go-atlas/security/secrets/providers/memory"
)

type sealedKey struct{ material []byte }

func TestNew_UncloneablePayload(t *testing.T) {
	t.Parallel()
	provider, err := memory.New[sealedKey](nil)
	require.NoError(t, err)

	_, err = secrets.New[sealedKey](provider)
	require.ErrorIs(t, err, secrets.ErrUncloneablePayload)

	mgr, err := secrets.New[sealedKey](provider, secrets.WithAllowShallowClone())
	require.NoError(t, err)
	require.NotNil(t, mgr)
}

// With an interface payload type the check moves to the values: an unsafe
// value is refused on Save and on fetch, skipped by the update cycle without
// evicting or reporting the key as deleted, while safe values still flow.
func TestManager_InterfacePayloadValuesChecked(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	provider, err := memory.New[any](map[string]any{
		"good": map[string]any{"user": "u"},
		"bad":  "plain",
	})
	require.NoError(t, err)
	mgr, err := secrets.New[any](provider)
	require.NoError(t, err)

	require.ErrorIs(t, mgr.Save(ctx, "new", sealedKey{material: []byte("k")}), secrets.ErrUncloneablePayload)

	watcher, err := mgr.Watch(ctx, secrets.WatchOptions{SkipInitialEvents: true})
	require.NoError(t, err)
	t.Cleanup(watcher.Stop)
	require.NoError(t, mgr.RunUpdateCycle(ctx))

	// "bad" turns unsafe at the provider; a new key marks the cycle's events.
	require.NoError(t, provider.Save(ctx, "bad", map[string]any{"ch": make(chan int)}))
	require.NoError(t, provider.Save(ctx, "marker", "m"))
	require.NoError(t, mgr.RunUpdateCycle(ctx))

	var events []secrets.WatchEvent[any]
	for len(events) == 0 || len(watcher.Events) > 0 {
		events = append(events, <-watcher.Events)
	}
	for _, ev := range events {
		require.NotEqual(t, "bad", ev.Key, "an unusable listed value is neither updated nor deleted: %v", ev.Type)
	}

	got, err := mgr.Value(ctx, "good", false)
	require.NoError(t, err)
	require.Equal(t, map[string]any{"user": "u"}, got.Value)

	cached, err := mgr.Value(ctx, "bad", false)
	require.NoError(t, err, "the last safe version stays cached")
	require.Equal(t, "plain", cached.Value)

	// A cache miss fetched from the provider meets the unsafe value.
	require.NoError(t, provider.Save(ctx, "fresh", map[string]any{"fn": func() {}}))
	_, err = mgr.Value(ctx, "fresh", true)
	require.ErrorIs(t, err, secrets.ErrUncloneablePayload)
}

// A self-referential slice payload passes the check and is saved.
func TestManager_SaveCyclicInterfacePayload(t *testing.T) {
	t.Parallel()
	provider, err := memory.New[any](nil)
	require.NoError(t, err)
	mgr, err := secrets.New[any](provider)
	require.NoError(t, err)

	cyclic := make([]any, 1)
	cyclic[0] = cyclic
	require.NoError(t, mgr.Save(t.Context(), "cyclic", cyclic))
}

// A cache supplied with WithCache is checked for payloads that never passed
// the per-value check.
func TestNew_SuppliedCacheWithUnsafePayload(t *testing.T) {
	t.Parallel()
	cache, err := secrets.NewStandardCache[string, *secrets.Value[any]](10)
	require.NoError(t, err)
	cache.Put("k", secrets.NewValue[any]("k", sealedKey{material: []byte("x")}, nil, ""))
	provider, err := memory.New[any](nil)
	require.NoError(t, err)

	_, err = secrets.New[any](provider, secrets.WithCache(cache))
	require.ErrorIs(t, err, secrets.ErrUncloneablePayload)
}
