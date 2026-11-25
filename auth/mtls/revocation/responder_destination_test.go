// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package revocation_test

import (
	"crypto/x509"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/mtls/revocation"

	"golang.org/x/crypto/ocsp"
)

// The responder URL comes from the peer's certificate: by default the checker
// must not connect to a loopback or otherwise non-public address.
func TestDefaultClientRefusesPrivateResponder(t *testing.T) {
	t.Parallel()
	ca := makeCA(t)
	var hits atomic.Int32
	srv := responder(t, ca, ocsp.Good, &hits) // listens on 127.0.0.1

	tests := []struct {
		name string
		url  string
	}{
		{"IP literal", srv.URL},
		// The refusal applies to the resolved address, not to the URL text.
		{"host name resolving to loopback", strings.Replace(srv.URL, "127.0.0.1", "localhost", 1)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			leaf := makeLeaf(t, ca, 42, tc.url)

			err := revocation.New([]*x509.Certificate{ca.Cert}).Check(t.Context(), leaf)
			require.ErrorIs(t, err, revocation.ErrPrivateResponder)

			open := revocation.New([]*x509.Certificate{ca.Cert}, revocation.WithFailMode(revocation.FailOpen))
			require.NoError(t, open.Check(t.Context(), leaf), "fail-open keeps accepting an indeterminate status")
		})
	}
	require.Zero(t, hits.Load(), "the responder must never be contacted")
}

func TestInsecureAllowPrivateRespondersOptsOut(t *testing.T) {
	t.Parallel()
	ca := makeCA(t)
	var hits atomic.Int32
	srv := responder(t, ca, ocsp.Good, &hits)
	leaf := makeLeaf(t, ca, 42, srv.URL)

	c := revocation.New([]*x509.Certificate{ca.Cert}, revocation.WithInsecureAllowPrivateResponders())
	require.NoError(t, c.Check(t.Context(), leaf))
	require.Equal(t, int32(1), hits.Load())
}

func TestAllowedResponders(t *testing.T) {
	t.Parallel()
	ca := makeCA(t)
	var hits atomic.Int32
	srv := responder(t, ca, ocsp.Good, &hits)
	u, err := url.Parse(srv.URL)
	require.NoError(t, err)

	tests := []struct {
		name    string
		allowed []string
		wantErr error
	}{
		{name: "host", allowed: []string{u.Hostname()}},
		{name: "host and port", allowed: []string{"ocsp.example.org", u.Host}},
		{name: "case-insensitive", allowed: []string{"LOCALHOST", " " + u.Hostname() + " "}},
		{name: "other host", allowed: []string{"ocsp.example.org"}, wantErr: revocation.ErrResponderNotAllowed},
		{name: "other port", allowed: []string{u.Hostname() + ":1"}, wantErr: revocation.ErrResponderNotAllowed},
	}
	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			leaf := makeLeaf(t, ca, int64(100+i), srv.URL)
			err := revocation.New([]*x509.Certificate{ca.Cert}, revocation.WithAllowedResponders(tc.allowed...)).Check(t.Context(), leaf)
			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func TestAllowedRespondersRefusalSendsNoRequest(t *testing.T) {
	t.Parallel()
	ca := makeCA(t)
	var hits atomic.Int32
	srv := responder(t, ca, ocsp.Good, &hits)
	leaf := makeLeaf(t, ca, 42, srv.URL)

	c := revocation.New([]*x509.Certificate{ca.Cert},
		revocation.WithInsecureAllowPrivateResponders(), revocation.WithAllowedResponders("ocsp.example.org"))
	require.ErrorIs(t, c.Check(t.Context(), leaf), revocation.ErrResponderNotAllowed)
	require.Zero(t, hits.Load())
}

func TestResponderSchemeMustBeHTTP(t *testing.T) {
	t.Parallel()
	ca := makeCA(t)

	for _, responderURL := range []string{
		"ftp://ocsp.example.org/",
		"file:///etc/passwd",
		"gopher://127.0.0.1:6379/_INFO",
		"ocsp.example.org",
		"http://",
	} {
		leaf := makeLeaf(t, ca, 42, responderURL)
		err := newChecker([]*x509.Certificate{ca.Cert}).Check(t.Context(), leaf)
		require.ErrorIs(t, err, revocation.ErrResponderNotAllowed, responderURL)
	}
}
