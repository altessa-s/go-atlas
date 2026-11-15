// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package revocation_test

import (
	"crypto/x509"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	"golang.org/x/crypto/ocsp"

	coremtls "github.com/altessa-s/go-atlas/auth/mtls"
)

// Two CAs with the same subject DN but different keys may issue the same
// serial number. A status cached for one certificate must not be served for
// the other.
func TestCacheKeyIncludesIssuerKey(t *testing.T) {
	t.Parallel()
	caA := testhelpers.NewCA(t, testhelpers.WithSubjectKeyID([]byte{0x0a}))
	caB := testhelpers.NewCA(t, testhelpers.WithSubjectKeyID([]byte{0x0b}))
	require.Equal(t, caA.Cert.RawSubject, caB.Cert.RawSubject)

	var hitsA, hitsB atomic.Int32
	leafA := makeLeaf(t, caA, 42, responder(t, caA, ocsp.Good, &hitsA).URL)
	leafB := makeLeaf(t, caB, 42, responder(t, caB, ocsp.Revoked, &hitsB).URL)

	c := newChecker([]*x509.Certificate{caA.Cert, caB.Cert})
	require.NoError(t, c.Check(leafA))
	require.ErrorIs(t, c.Check(leafB), coremtls.ErrRevoked)
	require.NoError(t, c.Check(leafA), "the revoked status of the other issuer's certificate must not leak back")
	require.Equal(t, int32(1), hitsA.Load())
	require.Equal(t, int32(1), hitsB.Load())
}
