// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mtls_test

import (
	"context"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/auth/mtls"

	coremtls "github.com/altessa-s/go-atlas/auth/mtls"
)

// A configuration without a trust decision fails at construction instead of
// answering 401 at runtime. A validator alone is not a decision.
func TestMiddlewareRequiresTrustDecision(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		opts []coremtls.Option
	}{
		{"no options", nil},
		{"ancillary validator", []coremtls.Option{coremtls.WithValidator(func(context.Context, *x509.Certificate) error { return nil })}},
		{"empty trust domain validator", []coremtls.Option{coremtls.WithValidator(coremtls.TrustDomainValidator())}},
		{"empty trust domains", []coremtls.Option{coremtls.WithTrustDomains()}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			mw, err := mtls.Middleware(tc.opts...)
			require.ErrorIs(t, err, coremtls.ErrNoTrustDomainPolicy)
			require.Nil(t, mw)
		})
	}
}

func TestMiddlewareWithTrustDecision(t *testing.T) {
	t.Parallel()
	foreign := func() *http.Request { return requestWithCert(t, spiffeCert(t, "spiffe://foreign.example/sa/billing")) }
	var got any

	pinned, err := mtls.Middleware(coremtls.WithTrustDomains("example.org"))
	require.NoError(t, err)
	rec := httptest.NewRecorder()
	pinned(capture(&got)).ServeHTTP(rec, foreign())
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	optIn, err := mtls.Middleware(coremtls.WithInsecureAnyTrustDomain())
	require.NoError(t, err)
	rec = httptest.NewRecorder()
	optIn(capture(&got)).ServeHTTP(rec, foreign())
	require.Equal(t, http.StatusOK, rec.Code)
}
