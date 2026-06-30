// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package spiffe sources rotating mutual-TLS configurations from the SPIFFE
// Workload API.
//
// # Overview
//
// The package is the producer half of mTLS: it obtains the service's own
// X.509-SVID and trust bundle from a SPIFFE Workload API endpoint (typically a
// SPIRE agent on a local Unix socket) and hands out *tls.Config values that
// stay current as the SVID rotates. It is the counterpart to the verifier-side
// stack in auth/spiffe and auth/mtls, which turn a peer's verified certificate
// into a principal; here the concern is keeping our own credentials fresh.
//
// # Source seam
//
// [Source] is the injection point. [workloadapi.X509Source] satisfies it and is
// what [New] wires up. Callers that already own a source, or tests, use
// [NewProvider].
//
// # Authorizer
//
// A peer authorizer is mandatory: without one the provider fails closed with
// [ErrNoAuthorizer] rather than trusting any SPIFFE ID. Build one with the
// helpers in the go-spiffe tlsconfig package, or derive it from configuration
// through the factory subpackage.
//
// # Usage
//
//	provider, err := spiffe.New(ctx,
//		spiffe.WithSocketPath("unix:///run/spire/agent/api.sock"),
//		spiffe.WithAuthorizer(tlsconfig.AuthorizeMemberOf(td)),
//	)
//	if err != nil {
//		return err
//	}
//	defer provider.Close()
//
//	server := &http.Server{TLSConfig: provider.MTLSServerConfig()}
//
// # Scope
//
// Obtaining and rotating the SVID is all this package does. Turning a verified
// peer certificate into a principal is auth/mtls; parsing a SPIFFE ID out of a
// certificate is auth/spiffe.
package spiffe
