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
// until its NextUpdate, and rejects a revoked peer with
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
// Network revocation checks fail; [FailOpen] (default) keeps new connections
// flowing during a responder outage, [FailClosed] rejects anything it cannot
// confirm good. Pick per the cost of an outage versus the cost of briefly
// trusting a revoked peer.
//
// # Usage
//
//	checker := revocation.New(caCerts, revocation.WithFailMode(revocation.FailClosed))
//	authFn := grpcmtls.AuthFunc(coremtls.WithValidator(checker.Validator()))
//
// # Scope
//
// Live OCSP is the per-certificate mechanism that fits the validator shape. CRL
// polling — a periodic bulk fetch rather than a per-peer query — is a separate
// concern left for a future addition.
package revocation
