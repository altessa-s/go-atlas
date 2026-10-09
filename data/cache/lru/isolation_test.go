// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package lru_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/cache/lru"
)

func TestComputeUsesKeyEquality(t *testing.T) {
	t.Parallel()
	plain, err := lru.NewCache[any, any](16)
	require.NoError(t, err)
	for name, c := range map[string]lru.Cacher[any, any]{"plain": plain, "expiring": lru.NewExpirableCache[any, any](16, time.Minute)} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			entered, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
			go func() {
				_, err := c.GetOrCompute(t.Context(), 1, func(context.Context) (any, error) {
					close(entered)
					<-release
					return "integer", nil
				})
				done <- err
			}()
			<-entered
			t.Cleanup(func() { close(release); <-done })
			v, err := c.GetOrCompute(t.Context(), "1", func(context.Context) (any, error) { return nil, nil })
			require.NoError(t, err)
			require.Nil(t, v)
			v, ok := c.Get("1")
			require.True(t, ok)
			require.Nil(t, v)
			ctx, cancel := context.WithTimeout(t.Context(), time.Millisecond)
			defer cancel()
			_, err = c.GetOrCompute(ctx, 1, func(context.Context) (any, error) { panic("duplicate") })
			require.ErrorIs(t, err, context.DeadlineExceeded)
		})
	}
}

type mutableStringer struct{ value string }

func (k *mutableStringer) String() string { return k.value }

func TestShardingUsesPointerIdentity(t *testing.T) {
	t.Parallel()
	c, err := lru.NewShardedCache[*mutableStringer, int](64)
	require.NoError(t, err)
	key := &mutableStringer{value: "before"}
	c.Put(key, 42)
	key.value = "after"
	got, ok := c.Get(key)
	require.True(t, ok)
	require.Equal(t, 42, got)
}
