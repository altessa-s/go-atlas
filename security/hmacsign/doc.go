// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package hmacsign signs and verifies HTTP request bodies with a shared-secret
// HMAC, the scheme webhook providers such as Stripe and GitHub use to prove a
// request came from them and was not tampered with in transit.
//
// It is the body-signing counterpart to the token packages: auth/oidc and
// auth/jwt authenticate a bearer *token*, this authenticates the *payload*. A
// receiver verifies an inbound webhook; a sender signs an outbound one.
//
// # Schemes
//
// A [Scheme] encodes one provider's wire format: the header the signature lives
// in, how the signed message is built from the timestamp and body, and how the
// header value is parsed and formatted. Two are built in:
//
//   - [GitHub] — header "X-Hub-Signature-256", value "sha256=<hex>", HMAC over
//     the raw body. No timestamp.
//   - [Stripe] — header "Stripe-Signature", value "t=<unix>,v1=<hex>", HMAC over
//     "<t>.<body>". The signed timestamp enables replay protection.
//
// Both use HMAC-SHA256 (the modern variant of each provider); legacy SHA-1 is
// out of scope.
//
// # Verifying
//
//	v := hmacsign.NewVerifier(hmacsign.Stripe(), secret)
//	if err := v.Verify(r.Header.Get(v.HeaderName()), body); err != nil {
//	    http.Error(w, "bad signature", http.StatusUnauthorized)
//	    return
//	}
//
// [Verifier.Verify] compares in constant time ([hmac.Equal]), rejects a Stripe
// timestamp outside the tolerance window ([WithTolerance], default five minutes)
// to blunt replays, and accepts any of several secrets ([WithSecrets]) so a key
// can be rotated without downtime.
//
// # Signing
//
//	s := hmacsign.NewSigner(hmacsign.Stripe(), secret)
//	req.Header.Set(s.HeaderName(), s.Sign(body))
//
// Read the raw body once and pass the same bytes to Verify/Sign: the HMAC is
// over the exact bytes, so any re-encoding breaks the signature.
package hmacsign
