// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package spiffe parses SPIFFE IDs from X.509 certificates. It is a pure,
// transport-free primitive: given a verified leaf certificate (or a raw URI), it
// returns the workload's [ID] — the trust domain and path of an
// "spiffe://trust-domain/path" identity.
//
// It carries no transport or TLS concern. A gRPC interceptor or any caller that
// already holds a verified peer certificate uses [IDFromCertificate] to turn it
// into an identity; the mTLS gRPC adapter in
// [github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth/mtls] wires
// this into the authentication seam.
//
// # Usage
//
//	id, err := spiffe.IDFromCertificate(cert) // cert is a *x509.Certificate
//	if err != nil {
//	    return err
//	}
//	_ = id.TrustDomain // "example.org"
//	_ = id.Path        // "/ns/default/sa/billing"
//	_ = id.String()    // "spiffe://example.org/ns/default/sa/billing"
package spiffe
