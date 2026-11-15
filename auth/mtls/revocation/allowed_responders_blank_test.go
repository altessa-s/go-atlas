// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package revocation_test

import (
	"crypto/x509"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/mtls/revocation"

	"golang.org/x/crypto/ocsp"
)

// A blank allowlist — an unset environment variable, for example — is no
// allowlist: it must neither refuse every responder nor lift the default
// address refusal.
func TestBlankAllowedRespondersIsNoAllowlist(t *testing.T) {
	t.Parallel()
	ca := makeCA(t)
	var hits atomic.Int32
	srv := responder(t, ca, ocsp.Good, &hits)

	for i, blank := range [][]string{{""}, {"  "}, {"", "\t"}} {
		leaf := makeLeaf(t, ca, int64(100+i), srv.URL)

		// The loopback responder is refused by the default guard, not by an allowlist.
		guarded := revocation.New([]*x509.Certificate{ca.Cert}, revocation.WithAllowedResponders(blank...))
		require.ErrorIs(t, guarded.Check(leaf), revocation.ErrPrivateResponder, "%q", blank)

		// With the guard off every responder is still a candidate.
		c := newChecker([]*x509.Certificate{ca.Cert}, revocation.WithAllowedResponders(blank...))
		require.NoError(t, c.Check(leaf), "%q", blank)
	}
}

func TestAllowedRespondersDropsBlankEntries(t *testing.T) {
	t.Parallel()
	ca := makeCA(t)
	var hits atomic.Int32
	srv := responder(t, ca, ocsp.Good, &hits)
	u, err := url.Parse(srv.URL)
	require.NoError(t, err)

	listed := revocation.New([]*x509.Certificate{ca.Cert}, revocation.WithAllowedResponders("", u.Hostname()))
	require.NoError(t, listed.Check(makeLeaf(t, ca, 42, srv.URL)))

	other := revocation.New([]*x509.Certificate{ca.Cert}, revocation.WithAllowedResponders("", "ocsp.example.org"))
	require.ErrorIs(t, other.Check(makeLeaf(t, ca, 43, srv.URL)), revocation.ErrResponderNotAllowed)
}
