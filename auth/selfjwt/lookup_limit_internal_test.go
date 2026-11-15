// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package selfjwt

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-core/runtime/singleflight"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	jwt "github.com/altessa-s/go-atlas/auth/jwt"
)

// TestPublicKeyLookupsAreBounded pins that distinct (subject, kid) pairs from
// unauthenticated tokens cannot open unbounded concurrent provider lookups:
// past WithMaxConcurrentLookups a new pair fails fast.
func TestPublicKeyLookupsAreBounded(t *testing.T) {
	t.Parallel()
	pub, _ := testhelpers.GenerateEd25519Key(t)
	p := &keyedGatedProvider{
		gated:   [2]string{"a", "k"},
		entered: make(chan struct{}),
		release: make(chan struct{}),
		keys: map[[2]string]jwt.VerificationKey{
			{"a", "k"}: {Algorithm: jwt.AlgEdDSA, Key: pub},
			{"b", "k"}: {Algorithm: jwt.AlgEdDSA, Key: pub},
		},
		calls: make(map[[2]string]int),
	}
	v := NewVerifier(p, "selfjwt-test", "svc", WithMaxConcurrentLookups(1))

	first := asyncPublicKey(t.Context(), v, "a", "k")
	<-p.entered

	_, err := v.publicKey(t.Context(), "b", "k")
	require.ErrorIs(t, err, singleflight.ErrOverloaded)

	close(p.release)
	r := <-first
	require.NoError(t, r.err)

	_, err = v.publicKey(t.Context(), "b", "k")
	require.NoError(t, err)
}
