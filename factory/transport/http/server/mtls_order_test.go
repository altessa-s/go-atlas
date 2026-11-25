// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-net/clientip"
	"github.com/altessa-s/go-atlas/auth/mtls"
	"github.com/altessa-s/go-atlas/auth/principal"
	"github.com/altessa-s/go-atlas/coord/limiters"
	"github.com/altessa-s/go-atlas/internal/testhelpers/mtlstest"
	"github.com/altessa-s/go-atlas/reliability/idempotency"
	"github.com/altessa-s/go-atlas/reliability/idempotency/principalowner"
	"github.com/altessa-s/go-atlas/reliability/idempotency/storages/memory"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/limiter"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/realip"
	"github.com/altessa-s/go-atlas/transport/http/server/writer"

	httpconfig "github.com/altessa-s/go-atlas/config/http"
	idem "github.com/altessa-s/go-atlas/transport/http/server/middlewares/idempotency"
)

func TestMTLSPrincipalBeforeLimiterAndIdempotency(t *testing.T) {
	t.Parallel()
	cfg := httpconfig.Default()
	ca := mtlstest.NewCA(t)
	keeper := idempotency.New(memory.New(), idempotency.WithOwner(principalowner.PrincipalOwner))
	extractor, err := clientip.NewExtractor()
	require.NoError(t, err)
	const alice = "spiffe://example.org/sa/alice"
	const bob = "spiffe://example.org/sa/bob"
	limit := limiters.Func(func(ctx context.Context) (*limiters.LimitInfo, error) {
		p, ok := principal.FromContext(ctx)
		if !ok || (p.Subject != alice && p.Subject != bob) {
			return nil, limiters.ErrLimitExceeded
		}
		return &limiters.LimitInfo{Limit: 10, Remaining: 9}, nil
	})
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{ca.ServerCert(t, net.IPv4(127, 0, 0, 1))},
		ClientCAs:    ca.Pool, ClientAuth: tls.RequireAndVerifyClientCert,
		MinVersion: tls.VersionTLS13,
	}
	srv, err := New(&cfg).WithListenAddress("127.0.0.1:0").WithTLSConfig(tlsConfig).
		WithMiddleware(realip.New(extractor, nil), limiter.New(limit), idem.New(keeper)).
		WithMTLSMiddleware(mtls.WithTrustDomains("example.org"), mtls.WithIdentity(func(cert *x509.Certificate) (any, error) {
			return principal.Principal{Subject: cert.URIs[0].String()}, nil
		})).Build()
	require.NoError(t, err)
	srv.Handle("/orders", func(rw writer.ReadWriter) { rw.ResponseWriter().WriteHeader(http.StatusCreated) }).Methods(http.MethodPost)
	require.NoError(t, srv.Start())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 5*time.Second)
		defer cancel()
		require.NoError(t, srv.Shutdown(ctx))
	})
	url := "https://" + srv.Listener().Addr().String() + "/orders"
	for _, tc := range []struct {
		subject string
		want    int
	}{{alice, http.StatusCreated}, {bob, http.StatusForbidden}} {
		client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{
			RootCAs: ca.Pool, Certificates: []tls.Certificate{ca.ClientCert(t, tc.subject)}, MinVersion: tls.VersionTLS13,
		}}}
		t.Cleanup(client.CloseIdleConnections)
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, url, http.NoBody)
		require.NoError(t, err)
		req.Header.Set("Idempotency-Key", "550e8400-e29b-41d4-a716-446655440000")
		resp, err := client.Do(req)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
		require.Equal(t, tc.want, resp.StatusCode, tc.subject)
	}
}

func TestMTLSMiddlewareRequiresTrustPolicy(t *testing.T) {
	t.Parallel()
	cfg := httpconfig.Default()
	_, err := New(&cfg).WithMTLSMiddleware().Build()
	require.ErrorIs(t, err, mtls.ErrNoTrustDomainPolicy)
}
