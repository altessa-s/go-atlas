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
)

// withoutSubjectKeyID returns a copy of a CA certificate that carries no
// SubjectKeyId, as an older CA certificate may.
func withoutSubjectKeyID(cert *x509.Certificate) *x509.Certificate {
	stripped := *cert
	stripped.SubjectKeyId = nil
	return &stripped
}

// CA key rollover: the old and the new CA share a subject DN and the old one
// has no SubjectKeyId. A leaf naming the new CA by key identifier must be
// matched to it wherever it stands in the issuer list.
func TestIssuerKeyIDMatchWinsOverEarlierSubjectMatch(t *testing.T) {
	t.Parallel()
	oldCA := testhelpers.NewCA(t)
	newCA := testhelpers.NewCA(t, testhelpers.WithSubjectKeyID([]byte{0x0b}))
	require.Equal(t, oldCA.Cert.RawSubject, newCA.Cert.RawSubject)

	var hits atomic.Int32
	leaf := makeLeaf(t, newCA, 42, responder(t, newCA, ocsp.Good, &hits).URL)

	c := newChecker([]*x509.Certificate{withoutSubjectKeyID(oldCA.Cert), newCA.Cert})
	require.NoError(t, c.Check(leaf))
	require.Equal(t, int32(1), hits.Load())
}

// Without a deciding key identifier, the issuer among those sharing the
// subject DN is the one whose key signed the leaf, not the first listed.
func TestIssuerChosenBySignatureAmongSharedSubject(t *testing.T) {
	t.Parallel()
	oldCA := testhelpers.NewCA(t)
	newCA := testhelpers.NewCA(t)
	issuers := []*x509.Certificate{withoutSubjectKeyID(oldCA.Cert), withoutSubjectKeyID(newCA.Cert)}

	for _, ca := range []*testhelpers.CA{oldCA, newCA} {
		var hits atomic.Int32
		leaf := *makeLeaf(t, ca, 42, responder(t, ca, ocsp.Good, &hits).URL)
		leaf.AuthorityKeyId = nil

		require.NoError(t, newChecker(issuers).Check(&leaf))
		require.Equal(t, int32(1), hits.Load())
	}
}
