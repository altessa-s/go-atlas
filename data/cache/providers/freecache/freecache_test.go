// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package freecache_test

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/cache/providers"
	"github.com/altessa-s/go-atlas/data/cache/providers/freecache"
)

func TestNew_Default(t *testing.T) {
	p := freecache.New()
	require.NotNil(t, p)
}

func TestNew_WithMaxSize(t *testing.T) {
	p := freecache.New(freecache.WithMaxSize(1024 * 1024))
	require.NotNil(t, p)
}

func TestProvider_SaveAndGet(t *testing.T) {
	p := freecache.New()
	ctx := t.Context()

	key := "test-key"
	value := []byte("test-value")

	err := p.Save(ctx, key, value, 10*time.Second)
	require.NoError(t, err)

	got, err := p.Get(ctx, key)
	require.NoError(t, err)
	require.Equal(t, string(value), string(got))
}

func TestProvider_Get_Missing(t *testing.T) {
	p := freecache.New()
	ctx := t.Context()

	_, err := p.Get(ctx, "nonexistent-key")
	require.ErrorIs(t, err, providers.ErrMissing)
}

func TestProvider_Exists_True(t *testing.T) {
	p := freecache.New()
	ctx := t.Context()

	key := "test-key"
	value := []byte("test-value")

	err := p.Save(ctx, key, value, 10*time.Second)
	require.NoError(t, err)

	exists, err := p.Exists(ctx, key)
	require.NoError(t, err)
	require.True(t, exists, "expected key to exist")
}

func TestProvider_Exists_False(t *testing.T) {
	p := freecache.New()
	ctx := t.Context()

	exists, err := p.Exists(ctx, "nonexistent-key")
	require.NoError(t, err)
	require.False(t, exists, "expected key to not exist")
}

func TestProvider_Delete(t *testing.T) {
	p := freecache.New()
	ctx := t.Context()

	key := "test-key"
	value := []byte("test-value")

	err := p.Save(ctx, key, value, 10*time.Second)
	require.NoError(t, err)

	err = p.Delete(ctx, key)
	require.NoError(t, err)

	_, err = p.Get(ctx, key)
	require.ErrorIs(t, err, providers.ErrMissing)
}

func TestProvider_DeleteMany(t *testing.T) {
	p := freecache.New()
	ctx := t.Context()

	keys := []string{"key1", "key2", "key3"}
	value := []byte("test-value")

	for _, key := range keys {
		err := p.Save(ctx, key, value, 10*time.Second)
		require.NoError(t, err)
	}

	err := p.DeleteMany(ctx, keys...)
	require.NoError(t, err)

	for _, key := range keys {
		_, err := p.Get(ctx, key)
		require.ErrorIs(t, err, providers.ErrMissing)
	}
}

func TestProvider_SaveWithTTL(t *testing.T) {
	p := freecache.New()
	ctx := t.Context()

	key := "test-key"
	value := []byte("test-value")

	err := p.Save(ctx, key, value, 1*time.Second)
	require.NoError(t, err)
}

func TestProvider_Concurrent(t *testing.T) {
	p := freecache.New()
	ctx := t.Context()

	const numGoroutines = 100
	var wg sync.WaitGroup

	for i := range numGoroutines {
		wg.Go(func() {
			key := string(rune('a' + (i % 26)))
			value := []byte("value-" + string(rune('0'+(i%10))))
			err := p.Save(ctx, key, value, 10*time.Second)
			if err != nil {
				t.Errorf("failed to save concurrently: %v", err)
			}
		})

		wg.Go(func() {
			key := string(rune('a' + (i % 26)))
			_, _ = p.Get(ctx, key)
		})
	}

	wg.Wait()
}
