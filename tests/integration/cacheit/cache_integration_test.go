// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cacheit_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/cache"
)

// user is the value the facade tests cache.
type user struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// stored decodes what the facade wrote to Redis under key.
func (f fixture) stored(tb testing.TB, key string) user {
	tb.Helper()
	raw, err := f.client.Get(tb.Context(), f.rawKey(key)).Bytes()
	require.NoError(tb, err)
	var u user
	require.NoError(tb, json.Unmarshal(raw, &u))
	return u
}

// TestCache_RoundTrip pins the facade over Redis: a value saved through it
// is stored as JSON and read back, and Exists, Delete and DeleteMany act on
// the same keys.
func TestCache_RoundTrip(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	c := cache.New(f.storage)
	ctx := t.Context()
	alice := user{ID: 1, Name: "Alice"}

	require.NoError(t, c.Save(ctx, "user:1", &alice))
	require.Equal(t, alice, f.stored(t, "user:1"))

	var got user
	require.NoError(t, c.Get(ctx, "user:1", &got))
	require.Equal(t, alice, got)
	exists, err := c.Exists(ctx, "user:1")
	require.NoError(t, err)
	require.True(t, exists)

	require.NoError(t, c.Save(ctx, "user:2", &user{ID: 2}))
	require.NoError(t, c.Save(ctx, "user:3", &user{ID: 3}))
	require.NoError(t, c.Delete(ctx, "user:1"))
	require.NoError(t, c.DeleteMany(ctx, "user:2", "user:3"))
	for _, key := range []string{"user:1", "user:2", "user:3"} {
		require.ErrorIs(t, c.Get(ctx, key, &got), cache.ErrMissing, key)
	}
}

// TestCache_TTL pins how the facade resolves a TTL into the lifetime of the
// Redis key: the default when none is given or TTLUseDefault is, no expiry
// for NoTTL, and an explicit TTL ages the key out on the server.
func TestCache_TTL(t *testing.T) {
	t.Parallel()
	const defaultTTL = 10 * time.Minute
	f := newFixture(t)
	c := cache.New(f.storage, cache.WithTTL(defaultTTL))
	ctx := t.Context()
	v := &user{ID: 1}

	require.NoError(t, c.Save(ctx, "default", v))
	require.NoError(t, c.Save(ctx, "use-default", v, cache.TTLUseDefault))
	for _, key := range []string{"default", "use-default"} {
		require.Greater(t, f.pttl(t, key), defaultTTL-time.Minute, key)
		require.LessOrEqual(t, f.pttl(t, key), defaultTTL, key)
	}

	require.NoError(t, c.Save(ctx, "forever", v, cache.NoTTL))
	require.Equal(t, time.Duration(-1), f.pttl(t, "forever"))

	require.NoError(t, c.Save(ctx, "short", v, shortTTL))
	f.awaitGone(t, "short")
	var got user
	require.ErrorIs(t, c.Get(ctx, "short", &got), cache.ErrMissing)
}

// TestCache_GetWithFallbackRunsTheFallbackOnce races many callers for one
// missing key. The fallback is held until every caller has missed, so all of
// them are in flight at once: it must run once, store its value once with
// its own TTL, and hand every caller the same value.
func TestCache_GetWithFallbackRunsTheFallbackOnce(t *testing.T) {
	t.Parallel()
	const callers = 32
	f := newFixture(t)
	counting := &countingStorage{Storage: f.storage}
	c := cache.New(counting)

	var calls atomic.Int32
	fallback := func(context.Context) (any, time.Duration, error) {
		calls.Add(1)
		// Every caller has missed once it is counted; a short grace covers
		// the step from the miss into the singleflight group. This runs off
		// the test goroutine, so it reports a timeout as an error.
		deadline := time.Now().Add(settleWindow)
		for counting.misses.Load() < callers {
			if time.Now().After(deadline) {
				return nil, 0, fmt.Errorf("only %d of %d callers missed", counting.misses.Load(), callers)
			}
			time.Sleep(time.Millisecond)
		}
		time.Sleep(50 * time.Millisecond)
		return &user{ID: 7, Name: "computed"}, 5 * time.Minute, nil
	}

	results := make([]user, callers)
	errs := make([]error, callers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range callers {
		wg.Go(func() {
			<-start
			errs[i] = c.GetWithFallback(t.Context(), "user:7", &results[i], fallback)
		})
	}
	close(start)
	wg.Wait()

	require.EqualValues(t, 1, calls.Load(), "the fallback must run once for all concurrent callers")
	for i := range callers {
		require.NoError(t, errs[i], "caller %d", i)
		require.Equal(t, user{ID: 7, Name: "computed"}, results[i], "caller %d", i)
	}
	require.Equal(t, user{ID: 7, Name: "computed"}, f.stored(t, "user:7"))
	require.Greater(t, f.pttl(t, "user:7"), 4*time.Minute, "the fallback's TTL must reach Redis")

	// Now cached: neither variant runs a fallback again.
	got, err := cache.GetWithFallbackT(t.Context(), c, "user:7", func(context.Context) (user, time.Duration, error) {
		calls.Add(1)
		return user{}, cache.TTLUseDefault, nil
	})
	require.NoError(t, err)
	require.Equal(t, user{ID: 7, Name: "computed"}, got)
	require.EqualValues(t, 1, calls.Load())
}

// TestCache_NegativeCaching pins the negative entry in Redis: a fallback
// reporting "not found" — by ErrMissing or by a nil value — leaves a short
// sentinel that answers lookups without the fallback until it expires.
func TestCache_NegativeCaching(t *testing.T) {
	t.Parallel()
	for name, notFound := range map[string]func(context.Context) (any, time.Duration, error){
		"ErrMissing": func(context.Context) (any, time.Duration, error) { return nil, 0, cache.ErrMissing },
		"NilValue":   func(context.Context) (any, time.Duration, error) { return (*user)(nil), 0, nil },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			c := cache.New(f.storage, cache.WithNegativeTTL(shortTTL))
			ctx := t.Context()

			var calls atomic.Int32
			fallback := func(ctx context.Context) (any, time.Duration, error) {
				calls.Add(1)
				return notFound(ctx)
			}
			var got user
			require.ErrorIs(t, c.GetWithFallback(ctx, "user:404", &got, fallback), cache.ErrMissing)
			require.Positive(t, f.pttl(t, "user:404"), "the negative entry must be stored with an expiry")
			require.LessOrEqual(t, f.pttl(t, "user:404"), shortTTL)

			require.ErrorIs(t, c.GetWithFallback(ctx, "user:404", &got, fallback), cache.ErrMissing)
			require.ErrorIs(t, c.Get(ctx, "user:404", &got), cache.ErrMissing)
			exists, err := c.Exists(ctx, "user:404")
			require.NoError(t, err)
			require.False(t, exists, "a negative entry is not a value")
			require.EqualValues(t, 1, calls.Load(), "a negative entry must answer without the fallback")

			f.awaitGone(t, "user:404")
			require.NoError(t, c.GetWithFallback(ctx, "user:404", &got, func(context.Context) (any, time.Duration, error) {
				calls.Add(1)
				return &user{ID: 404, Name: "created"}, cache.TTLUseDefault, nil
			}))
			require.Equal(t, user{ID: 404, Name: "created"}, got)
			require.EqualValues(t, 2, calls.Load())
		})
	}
}

// TestCache_FailedFallbackIsNotCached pins that only "not found" is cached
// negatively: any other fallback error leaves Redis untouched, so the next
// call tries again.
func TestCache_FailedFallbackIsNotCached(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	c := cache.New(f.storage, cache.WithNegativeTTL(time.Minute))
	errDown := errors.New("database down")

	var got user
	err := c.GetWithFallback(t.Context(), "user:1", &got, func(context.Context) (any, time.Duration, error) {
		return nil, 0, errDown
	})
	require.ErrorIs(t, err, errDown)
	require.Equal(t, time.Duration(-2), f.pttl(t, "user:1"), "a failed fallback must not write")

	require.NoError(t, c.GetWithFallback(t.Context(), "user:1", &got, func(context.Context) (any, time.Duration, error) {
		return &user{ID: 1}, cache.TTLUseDefault, nil
	}))
	require.Equal(t, user{ID: 1}, f.stored(t, "user:1"))
}

type tenantKey struct{}

// TestCache_NamespacesIsolateTenants pins WithKeyNamespace against the keys
// it produces in Redis: two tenants reading the same key compute and store
// separate values.
func TestCache_NamespacesIsolateTenants(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	c := cache.New(f.storage, cache.WithKeyNamespace(func(ctx context.Context) string {
		v, _ := ctx.Value(tenantKey{}).(string)
		return v
	}))

	for _, tenant := range []string{"acme", "globex"} {
		ctx := context.WithValue(t.Context(), tenantKey{}, tenant)
		got, err := cache.GetWithFallbackT(ctx, c, "settings", func(context.Context) (user, time.Duration, error) {
			return user{Name: tenant}, cache.TTLUseDefault, nil
		})
		require.NoError(t, err)
		require.Equal(t, tenant, got.Name, "a tenant must not read another's entry")
		require.Equal(t, user{Name: tenant}, f.stored(t, fmt.Sprintf("%d:%s:settings", len(tenant), tenant)))
	}
}
