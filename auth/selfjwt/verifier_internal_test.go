// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package selfjwt

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

// countingProvider counts VerificationKey calls so a test can observe how often
// the key provider is hit under concurrent cache misses.
type countingProvider struct {
	mu    sync.Mutex
	calls int
	vk    VerificationKey
}

func (p *countingProvider) SigningKey(context.Context, string) (SigningKey, error) {
	return SigningKey{}, ErrSubjectUnknown
}

func (p *countingProvider) VerificationKey(context.Context, string, string) (VerificationKey, error) {
	p.mu.Lock()
	p.calls++
	p.mu.Unlock()
	return p.vk, nil
}

func (p *countingProvider) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

// TestPublicKeyConcurrentMissesAreConsistent fans out concurrent misses for the
// same (subject, kid) through the singleflight path and asserts every caller
// receives the resolved key with no error. Run under -race it also guards the
// singleflight wiring against data races and result-type assertion panics.
func TestPublicKeyConcurrentMissesAreConsistent(t *testing.T) {
	t.Parallel()
	pub, _ := testhelpers.GenerateEd25519Key(t)

	p := &countingProvider{vk: VerificationKey{Algorithm: AlgEdDSA, Key: pub}}
	v := NewVerifier(p)

	const goroutines = 64
	got := make([]VerificationKey, goroutines)
	errs := make([]error, goroutines)
	var wg sync.WaitGroup
	for i := range goroutines {
		wg.Go(func() {
			got[i], errs[i] = v.publicKey(t.Context(), "svc", "k1")
		})
	}
	wg.Wait()

	for i := range goroutines {
		require.NoErrorf(t, errs[i], "goroutine %d", i)
		require.Equalf(t, AlgEdDSA, got[i].Algorithm, "goroutine %d", i)
	}
	// The provider is consulted at least once; the cache serves every caller
	// after the first resolution regardless of how the flights interleave.
	require.GreaterOrEqual(t, p.callCount(), 1)
}

// gatedProvider blocks its first VerificationKey call until released and
// answers it with the pre-retirement key; every later call reports the kid as
// rotated away. It models a provider lookup that read the old key just before
// the key was retired and the verifier invalidated.
type gatedProvider struct {
	entered chan struct{}
	release chan struct{}
	calls   atomic.Int32
	old     VerificationKey
}

func (p *gatedProvider) SigningKey(context.Context, string) (SigningKey, error) {
	return SigningKey{}, ErrSubjectUnknown
}

func (p *gatedProvider) VerificationKey(context.Context, string, string) (VerificationKey, error) {
	if p.calls.Add(1) == 1 {
		close(p.entered)
		<-p.release
		return p.old, nil
	}
	return VerificationKey{}, ErrKeyRotated
}

func invalidateKey(v *Verifier)     { v.InvalidateKey("svc", "k1") }
func invalidateSubject(v *Verifier) { v.InvalidateSubject("svc") }

type lookupResult struct {
	vk  VerificationKey
	err error
}

// asyncPublicKey runs v.publicKey in its own goroutine and delivers the result
// on the returned channel, which is buffered so the goroutine never blocks.
func asyncPublicKey(ctx context.Context, v *Verifier, subject, kid string) <-chan lookupResult {
	ch := make(chan lookupResult, 1)
	go func() {
		vk, err := v.publicKey(ctx, subject, kid)
		ch <- lookupResult{vk: vk, err: err}
	}()
	return ch
}

// TestInvalidationFencesInFlightLookup asserts that an invalidation fences a
// provider lookup already in flight: a verification that starts after the
// invalidation does not join the stale flight, and the stale flight does not
// re-populate the cache with the retired key once it completes.
func TestInvalidationFencesInFlightLookup(t *testing.T) {
	t.Parallel()
	pub, _ := testhelpers.GenerateEd25519Key(t)

	tests := []struct {
		name       string
		ttl        time.Duration
		follower   bool
		invalidate func(v *Verifier)
	}{
		{name: "InvalidateKey", ttl: time.Minute, follower: true, invalidate: invalidateKey},
		{name: "InvalidateSubject", ttl: time.Minute, follower: true, invalidate: invalidateSubject},
		{name: "InvalidateKey_ZeroTTL", follower: true, invalidate: invalidateKey},
		{name: "InvalidateSubject_ZeroTTL", follower: true, invalidate: invalidateSubject},
		{name: "InvalidateKey_NoFollower", ttl: time.Minute, invalidate: invalidateKey},
		{name: "InvalidateSubject_NoFollower", ttl: time.Minute, invalidate: invalidateSubject},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := &gatedProvider{
				entered: make(chan struct{}),
				release: make(chan struct{}),
				old:     VerificationKey{Algorithm: AlgEdDSA, Key: pub},
			}
			release := sync.OnceFunc(func() { close(p.release) })
			t.Cleanup(release)
			v := NewVerifier(p, WithCacheTTL(tc.ttl))

			lookup := func() <-chan lookupResult { return asyncPublicKey(t.Context(), v, "svc", "k1") }

			stale := lookup()
			<-p.entered // the stale lookup has read the provider before retirement
			tc.invalidate(v)

			// A verification that starts after the invalidation must consult the
			// provider afresh rather than wait on, and share, the stale flight.
			if tc.follower {
				select {
				case r := <-lookup():
					require.ErrorIs(t, r.err, ErrKeyRotated)
				case <-time.After(5 * time.Second):
					t.Fatal("post-invalidation lookup joined the in-flight pre-invalidation lookup")
				}
			}

			release()
			r := <-stale
			require.NoError(t, r.err, "a lookup started before invalidation may still return the old key")
			require.Equal(t, AlgEdDSA, r.vk.Algorithm)

			// The stale flight must not have re-cached the retired key.
			_, ok := v.cache.get("svc", "k1")
			require.False(t, ok, "stale lookup re-cached the retired key")
			_, err := v.publicKey(t.Context(), "svc", "k1")
			require.ErrorIs(t, err, ErrKeyRotated)
		})
	}
}

func TestFlightKeyIsInjective(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		a, b [2]string
		genA uint64
		genB uint64
	}{
		{name: "NULInKidVsSubject", a: [2]string{"a", "b\x00c"}, b: [2]string{"a\x00b", "c"}},
		{name: "SplitPoint", a: [2]string{"ab", "c"}, b: [2]string{"a", "bc"}},
		{name: "DigitsAndColons", a: [2]string{"1:a", "1"}, b: [2]string{"1", ":a1"}},
		{name: "KidDigitsVsGeneration", a: [2]string{"s", "k1"}, b: [2]string{"s", "k"}, genA: 0, genB: 10},
		{name: "Generation", a: [2]string{"s", "k"}, b: [2]string{"s", "k"}, genA: 1, genB: 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.NotEqual(t,
				flightKey(tc.a[0], tc.a[1], tc.genA),
				flightKey(tc.b[0], tc.b[1], tc.genB))
		})
	}
	require.Equal(t, flightKey("s", "k", 3), flightKey("s", "k", 3))
}

// keyedGatedProvider resolves a distinct key per (subject, kid) and blocks the
// lookup for gated until released, so a test can hold one flight open while a
// lookup for another pair runs.
type keyedGatedProvider struct {
	gated   [2]string
	entered chan struct{}
	release chan struct{}
	keys    map[[2]string]VerificationKey
	mu      sync.Mutex
	calls   map[[2]string]int
}

func (p *keyedGatedProvider) SigningKey(context.Context, string) (SigningKey, error) {
	return SigningKey{}, ErrSubjectUnknown
}

func (p *keyedGatedProvider) VerificationKey(_ context.Context, subject, kid string) (VerificationKey, error) {
	pair := [2]string{subject, kid}
	p.mu.Lock()
	p.calls[pair]++
	p.mu.Unlock()
	if pair == p.gated {
		close(p.entered)
		<-p.release
	}
	vk, ok := p.keys[pair]
	if !ok {
		return VerificationKey{}, ErrSubjectUnknown
	}
	return vk, nil
}

// TestPublicKeyFlightKeyIsInjective asserts that lookups for distinct
// (subject, kid) pairs whose naive NUL-joined encodings collide never share a
// flight: each pair is resolved by its own provider call to its own key.
func TestPublicKeyFlightKeyIsInjective(t *testing.T) {
	t.Parallel()
	pubA, _ := testhelpers.GenerateEd25519Key(t)
	pubB, _ := testhelpers.GenerateEd25519Key(t)
	pairA := [2]string{"a", "b\x00c"}
	pairB := [2]string{"a\x00b", "c"}
	p := &keyedGatedProvider{
		gated:   pairA,
		entered: make(chan struct{}),
		release: make(chan struct{}),
		keys: map[[2]string]VerificationKey{
			pairA: {Algorithm: AlgEdDSA, Key: pubA},
			pairB: {Algorithm: AlgEdDSA, Key: pubB},
		},
		calls: make(map[[2]string]int),
	}
	release := sync.OnceFunc(func() { close(p.release) })
	t.Cleanup(release)
	v := NewVerifier(p)

	first := asyncPublicKey(t.Context(), v, pairA[0], pairA[1])
	<-p.entered

	second := asyncPublicKey(t.Context(), v, pairB[0], pairB[1])
	select {
	case r := <-second:
		require.NoError(t, r.err)
		require.Equal(t, pubB, r.vk.Key)
	case <-time.After(5 * time.Second):
		t.Fatal("lookup for a distinct (subject, kid) joined another pair's flight")
	}

	release()
	r := <-first
	require.NoError(t, r.err)
	require.Equal(t, pubA, r.vk.Key)

	p.mu.Lock()
	defer p.mu.Unlock()
	require.Equal(t, 1, p.calls[pairA])
	require.Equal(t, 1, p.calls[pairB])
}
