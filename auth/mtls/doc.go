// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package mtls turns a verified mutual-TLS client certificate into an
// authenticated principal. It is the transport-free policy core behind the gRPC
// and HTTP mTLS adapters: an [Authenticator] derives the identity from the
// certificate, runs extra validators, and records an audit decision. The
// transport adapters only extract the verified certificate and map a failure to
// their status code.
//
// The default identity is the certificate's SPIFFE ID (see
// [github.com/altessa-s/go-atlas/auth/spiffe]); override it with [WithIdentity].
//
// # Validation beyond TLS
//
// The TLS handshake already verifies the certificate chain and validity window
// at connection time. Validators added with [WithValidator] run on every call,
// which matters for long-lived connections: [ExpiryValidator] re-checks the
// validity window so a connection that outlives its certificate is rejected, and
// [TrustDomainValidator] pins the accepted SPIFFE trust domains. A revocation
// lookup (CRL/OCSP/denylist) is a caller-supplied [CertValidator].
//
// # Usage
//
//	auth := mtls.NewAuthenticator(
//	    mtls.WithValidator(mtls.ExpiryValidator(nil, 30*time.Second)),
//	    mtls.WithValidator(mtls.TrustDomainValidator("example.org")),
//	    mtls.WithAudit(rec, func(p any) string { id, _ := p.(spiffe.ID); return id.String() }),
//	)
//	principal, err := auth.Authenticate(ctx, verifiedCert)
package mtls
