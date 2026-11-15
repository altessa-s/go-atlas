// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package mtls is the HTTP adapter for mutual-TLS authentication. [Middleware]
// reads the verified peer certificate from the request's TLS state, derives a
// principal through a [github.com/altessa-s/go-atlas/auth/mtls.Authenticator]
// built from opts, and installs it into the request context with
// [github.com/altessa-s/go-atlas/transport/http/server/middlewares/auth.ContextWithPrincipal]
// so downstream authorization (for example
// [github.com/altessa-s/go-atlas/transport/http/server/middlewares/auth.ScopeMiddleware])
// reads it via FromContext.
//
// Configure behavior with the core package's options (mtls.WithTrustDomains /
// WithIdentity / WithValidator / WithAudit). The options must make the trust
// decision — mtls.WithTrustDomains or the mtls.WithInsecureAnyTrustDomain
// opt-in; validators never count as one, and without it [Middleware] returns
// an error wrapping mtls.ErrNoTrustDomainPolicy instead of a middleware. This
// adapter sets the "http" transport label and
// maps a missing or rejected certificate to 401, a required-audit failure to
// 500. The certificate is read from the verified chain (populated only under
// tls.RequireAndVerifyClientCert), never the unverified PeerCertificates.
//
// # Usage
//
//	import (
//	    coremtls "github.com/altessa-s/go-atlas/auth/mtls"
//	    httpauth "github.com/altessa-s/go-atlas/transport/http/server/middlewares/auth"
//	    "github.com/altessa-s/go-atlas/transport/http/server/middlewares/auth/mtls"
//	)
//
//	authn, err := mtls.Middleware(coremtls.WithTrustDomains("example.org"))
//	if err != nil {
//	    return err
//	}
//	authz := httpauth.ScopeMiddleware(enf, keyFunc)
//	handler = authn(authz(handler)) // authn populates the principal authz reads
//
// The TLS configuration that requires and verifies client certificates is set up
// separately — see [github.com/altessa-s/go-tlsutils].
package mtls
