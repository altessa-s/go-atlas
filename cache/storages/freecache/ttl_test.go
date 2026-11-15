// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package freecache_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/cache/storages"
	"github.com/altessa-s/go-atlas/cache/storages/freecache"

	backend "github.com/coocood/freecache"
)

type cacheTimer struct{ now uint32 }

func (c *cacheTimer) Now() uint32 { return c.now }

func TestSaveFractionalTTL(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		ttl     time.Duration
		seconds uint32
	}{
		{"nanosecond", time.Nanosecond, 1},
		{"half_second", time.Second / 2, 1},
		{"fractional_seconds", time.Second + time.Nanosecond, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			clock := &cacheTimer{now: 100}
			p := &freecache.Storage{Cache: backend.NewCacheCustomTimer(1<<20, clock)}
			require.NoError(t, p.Save(t.Context(), "key", []byte("value"), tc.ttl))
			ttl, err := p.TTL([]byte("key"))
			require.NoError(t, err)
			require.Equal(t, tc.seconds, ttl)
			clock.now += tc.seconds
			_, err = p.Get(t.Context(), "key")
			require.ErrorIs(t, err, storages.ErrMissing)
		})
	}
}
