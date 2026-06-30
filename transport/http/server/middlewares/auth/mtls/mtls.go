// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mtls

import (
	"crypto/x509"
	"errors"
	"net/http"

	"github.com/altessa-s/go-atlas/auth/audit"

	coremtls "github.com/altessa-s/go-atlas/auth/mtls"
	authmw "github.com/altessa-s/go-atlas/transport/http/server/middlewares/auth"
)

var (
	// ErrNoTLS indicates the request did not arrive over TLS.
	ErrNoTLS = errors.New("mtls: request is not over TLS")
	// ErrNoVerifiedCert indicates the client presented no verified certificate —
	// the server was not configured to require and verify it.
	ErrNoVerifiedCert = errors.New("mtls: no verified client certificate")
)

// PeerCertificate returns the verified leaf client certificate from the request.
// It reads the verified chain, which the TLS stack populates only when the
// server requires and verifies the client certificate
// (tls.RequireAndVerifyClientCert) — never the unverified PeerCertificates.
func PeerCertificate(r *http.Request) (*x509.Certificate, error) {
	if r.TLS == nil {
		return nil, ErrNoTLS
	}
	chains := r.TLS.VerifiedChains
	if len(chains) == 0 || len(chains[0]) == 0 {
		return nil, ErrNoVerifiedCert
	}
	return chains[0][0], nil
}

// Middleware authenticates each request from its verified mTLS client
// certificate and installs the derived principal into the request context (read
// downstream with [authmw.FromContext], e.g. by [authmw.ScopeMiddleware]).
// Identity derivation, validation, and audit are delegated to a
// [github.com/altessa-s/go-atlas/auth/mtls.Authenticator] built from opts; the
// transport label is set to "http".
//
// A request without a verified client certificate, or one the authenticator
// rejects, is answered 401 Unauthorized. A required-audit failure on an
// otherwise-successful authentication is answered 500 so nothing proceeds
// unrecorded.
func Middleware(opts ...coremtls.Option) func(http.Handler) http.Handler {
	authenticator := coremtls.NewAuthenticator(append([]coremtls.Option{coremtls.WithTransport("http")}, opts...)...)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cert, err := PeerCertificate(r)
			if err != nil {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			principal, err := authenticator.Authenticate(r.Context(), cert)
			if err != nil {
				if errors.Is(err, audit.ErrAuditFailed) {
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
					return
				}
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			ctx := authmw.ContextWithPrincipal(r.Context(), principal)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
