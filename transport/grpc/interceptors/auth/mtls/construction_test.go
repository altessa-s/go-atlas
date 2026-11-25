// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mtls_test

import (
	"context"
	"crypto/x509"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth/mtls"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	coremtls "github.com/altessa-s/go-atlas/auth/mtls"
	authgrpc "github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"
)

// A configuration without a trust decision fails at construction instead of
// answering Unauthenticated at runtime. A validator alone is not a decision.
func TestAuthFuncRequiresTrustDecision(t *testing.T) {
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
			authFn, err := mtls.AuthFunc(tc.opts...)
			require.ErrorIs(t, err, coremtls.ErrNoTrustDomainPolicy)
			require.Nil(t, authFn)
		})
	}
}

func TestAuthFuncWithTrustDecision(t *testing.T) {
	t.Parallel()
	foreign := ctxWithVerifiedCert(t.Context(), spiffeCert(t, "spiffe://foreign.example/sa/billing"))

	pinned, err := mtls.AuthFunc(coremtls.WithTrustDomains("example.org"))
	require.NoError(t, err)
	_, err = pinned(foreign, authgrpc.Request{})
	require.Equal(t, codes.Unauthenticated, status.Code(err))

	optIn, err := mtls.AuthFunc(coremtls.WithInsecureAnyTrustDomain())
	require.NoError(t, err)
	_, err = optIn(foreign, authgrpc.Request{})
	require.NoError(t, err)
}
