// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package mtls is the gRPC adapter for mutual-TLS authentication. [AuthFunc]
// returns an [github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth.AuthFunc]
// that extracts the verified peer certificate from the connection and delegates
// identity derivation, validation, and audit to a
// [github.com/altessa-s/go-atlas/auth/mtls.Authenticator].
//
// Configure behavior with the core package's options
// (mtls.WithTrustDomains / WithIdentity / WithValidator / WithAudit). The
// options must make the trust decision — mtls.WithTrustDomains or the
// mtls.WithInsecureAnyTrustDomain opt-in; validators never count as one, and
// without it [AuthFunc] returns an error wrapping mtls.ErrNoTrustDomainPolicy
// instead of an auth function. This adapter only adds the
// transport label and maps failures to gRPC status codes — codes.Unauthenticated
// for an unauthenticated peer or a rejected certificate, codes.Internal for a
// required-audit failure. The verified certificate comes from the verified chain
// (populated only under tls.RequireAndVerifyClientCert), never the unverified
// PeerCertificates.
//
// # Usage
//
//	import (
//	    coremtls "github.com/altessa-s/go-atlas/auth/mtls"
//	    grpcauth "github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"
//	    "github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth/mtls"
//	)
//
//	authFn, err := mtls.AuthFunc(coremtls.WithTrustDomains("example.org"))
//	if err != nil {
//	    return err
//	}
//	interceptor := grpcauth.ServerInterceptor(
//	    grpcauth.WithAuthFn(authFn),
//	    grpcauth.WithClientAuth(grpcauth.ScopeClientAuth(enf)),
//	)
//
// The TLS configuration that requires and verifies client certificates is set up
// separately — see [github.com/altessa-s/go-tlsutils].
package mtls
