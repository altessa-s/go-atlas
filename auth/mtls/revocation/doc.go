// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package revocation provides a live OCSP peer-revocation check as an
// [github.com/altessa-s/go-atlas/auth/mtls.CertValidator].
//
// # Overview
//
// The TLS handshake verifies that a client certificate chains to a trusted CA,
// but it does not ask whether that certificate has since been revoked. This
// package closes that gap on the verifier side: a [Checker] queries the
// issuer's OCSP responder for the peer's leaf certificate, caches the answer
// until its NextUpdate (a "good" answer without one is trusted only up to
// [DefaultMaxResponseAge] after its ThisUpdate, see [WithMaxResponseAge]), and
// rejects a revoked peer with
// [github.com/altessa-s/go-atlas/auth/mtls.ErrRevoked].
//
// It complements the in-memory [github.com/altessa-s/go-atlas/auth/mtls.RevocationList]
// (a static serial denylist) with a network-backed, always-current source.
//
// # Issuers
//
// An OCSP request is keyed by the issuer's name and public key, which the
// single-leaf validator seam does not carry. The trusting issuer certificates
// are therefore supplied at construction — in mTLS these are the same CA
// certificates configured as ClientCAs/RootCAs.
//
// # Fail mode
//
// Network revocation checks fail; [FailClosed] (default) rejects anything it
// cannot confirm good, so blocking the responder does not let a revoked
// certificate through. [FailOpen] keeps new connections flowing during a
// responder outage and must be selected explicitly with [WithFailMode].
//
// # Responder destination
//
// The responder URL is read from the peer's certificate, so it is restricted:
// only http and https URLs are requested, and the default client refuses a
// responder that resolves to a loopback, private, link-local, unique-local,
// multicast, unspecified or otherwise non-public address
// ([ErrPrivateResponder]). The refusal runs on the resolved address right
// before the connect, so it covers redirects and DNS rebinding. An internal
// PKI whose responder lives on a private network names it with
// [WithAllowedResponders], which also limits the checker to the listed hosts
// ([ErrResponderNotAllowed]); [WithInsecureAllowPrivateResponders] turns the
// refusal off altogether.
//
// # Responder authorization
//
// A response must be signed with the issuer's own key or by a delegated
// responder certificate issued directly by that issuer, carrying the
// id-kp-OCSPSigning extended key usage and within its validity period (RFC
// 6960 section 4.2.2.2). Anything else is [ErrUnauthorizedResponder].
//
// # HTTP client
//
// The default OCSP client is bounded by the check timeout and refuses a
// redirect to a different host ([ErrCrossHostRedirect]), so the responder named
// in the certificate cannot steer the request elsewhere. A client supplied with
// [WithHTTPClient] is used as is: its timeout, redirects and destinations are
// the caller's responsibility.
//
// # Usage
//
//	checker := revocation.New(caCerts)
//	authFn, err := grpcmtls.AuthFunc(
//	    coremtls.WithTrustDomains("example.org"),
//	    coremtls.WithValidator(checker.Validator()),
//	)
//
// # Scope
//
// Live OCSP is the per-certificate mechanism that fits the validator shape. CRL
// polling — a periodic bulk fetch rather than a per-peer query — is a separate
// concern left for a future addition.
package revocation
