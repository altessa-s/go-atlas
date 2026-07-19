// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package selfjwt

import (
	"context"
	"sync"
	"testing"

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
