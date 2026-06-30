// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package spiffe_test

import (
	"crypto/x509"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/spiffe"
)

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	require.NoError(t, err)
	return u
}

func TestParseID(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		raw     string
		want    spiffe.ID
		wantErr bool
	}{
		{"workload", "spiffe://example.org/ns/default/sa/billing", spiffe.ID{TrustDomain: "example.org", Path: "/ns/default/sa/billing"}, false},
		{"bare trust domain", "spiffe://example.org", spiffe.ID{TrustDomain: "example.org"}, false},
		{"uppercase host lowered", "spiffe://Example.ORG/x", spiffe.ID{TrustDomain: "example.org", Path: "/x"}, false},
		{"wrong scheme", "https://example.org/x", spiffe.ID{}, true},
		{"missing trust domain", "spiffe:///x", spiffe.ID{}, true},
		{"with port", "spiffe://example.org:8443/x", spiffe.ID{}, true},
		{"with query", "spiffe://example.org/x?a=b", spiffe.ID{}, true},
		{"with fragment", "spiffe://example.org/x#f", spiffe.ID{}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := spiffe.ParseID(tc.raw)
			if tc.wantErr {
				require.ErrorIs(t, err, spiffe.ErrInvalidID)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestIDString(t *testing.T) {
	t.Parallel()
	require.Equal(t, "spiffe://example.org/x", spiffe.ID{TrustDomain: "example.org", Path: "/x"}.String())
	require.Equal(t, "", spiffe.ID{}.String())
	require.True(t, spiffe.ID{}.IsZero())
	require.False(t, spiffe.ID{TrustDomain: "example.org"}.IsZero())
}

func TestIDFromCertificate(t *testing.T) {
	t.Parallel()

	t.Run("single URI", func(t *testing.T) {
		t.Parallel()
		cert := &x509.Certificate{URIs: []*url.URL{mustURL(t, "spiffe://example.org/ns/default/sa/billing")}}
		id, err := spiffe.IDFromCertificate(cert)
		require.NoError(t, err)
		require.Equal(t, "spiffe://example.org/ns/default/sa/billing", id.String())
	})

	t.Run("nil cert", func(t *testing.T) {
		t.Parallel()
		_, err := spiffe.IDFromCertificate(nil)
		require.ErrorIs(t, err, spiffe.ErrNoSVID)
	})

	t.Run("no URI", func(t *testing.T) {
		t.Parallel()
		_, err := spiffe.IDFromCertificate(&x509.Certificate{})
		require.ErrorIs(t, err, spiffe.ErrNoSVID)
	})

	t.Run("multiple URIs", func(t *testing.T) {
		t.Parallel()
		cert := &x509.Certificate{URIs: []*url.URL{
			mustURL(t, "spiffe://example.org/a"),
			mustURL(t, "spiffe://example.org/b"),
		}}
		_, err := spiffe.IDFromCertificate(cert)
		require.ErrorIs(t, err, spiffe.ErrMultipleURIs)
	})

	t.Run("non-spiffe URI", func(t *testing.T) {
		t.Parallel()
		cert := &x509.Certificate{URIs: []*url.URL{mustURL(t, "https://example.org/a")}}
		_, err := spiffe.IDFromCertificate(cert)
		require.ErrorIs(t, err, spiffe.ErrInvalidID)
	})
}
