// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package selfjwt

import "github.com/altessa-s/go-atlas/auth/jwt"

// Algorithm is a JWA signature algorithm name (for example "EdDSA", "ES256",
// "RS256"). It aliases [github.com/altessa-s/go-atlas/auth/jwt.Algorithm] so the
// minter and verifier share one algorithm type with the embedded jwt engine
// without a conversion bridge. It is a plain string, so deployments can extend
// the set with any algorithm registered with golang-jwt via
// jwt.RegisterSigningMethod and pass it through [WithAllowedAlgorithms]; the
// constants below cover the asymmetric algorithms golang-jwt supports out of the
// box.
type Algorithm = jwt.Algorithm

// Asymmetric signature algorithms recognized out of the box, re-exported from
// [github.com/altessa-s/go-atlas/auth/jwt]. HMAC (HS256, ...) is intentionally
// not listed: a self-issued-token deployment verifies with a public key, and
// accepting a symmetric algorithm would invite key-confusion.
const (
	// AlgEdDSA is Ed25519 (EdDSA).
	AlgEdDSA = jwt.AlgEdDSA

	// AlgES256 is ECDSA using P-256 and SHA-256.
	AlgES256 = jwt.AlgES256
	// AlgES384 is ECDSA using P-384 and SHA-384.
	AlgES384 = jwt.AlgES384
	// AlgES512 is ECDSA using P-521 and SHA-512.
	AlgES512 = jwt.AlgES512

	// AlgRS256 is RSASSA-PKCS1-v1_5 using SHA-256.
	AlgRS256 = jwt.AlgRS256
	// AlgRS384 is RSASSA-PKCS1-v1_5 using SHA-384.
	AlgRS384 = jwt.AlgRS384
	// AlgRS512 is RSASSA-PKCS1-v1_5 using SHA-512.
	AlgRS512 = jwt.AlgRS512

	// AlgPS256 is RSASSA-PSS using SHA-256 and MGF1 with SHA-256.
	AlgPS256 = jwt.AlgPS256
	// AlgPS384 is RSASSA-PSS using SHA-384 and MGF1 with SHA-384.
	AlgPS384 = jwt.AlgPS384
	// AlgPS512 is RSASSA-PSS using SHA-512 and MGF1 with SHA-512.
	AlgPS512 = jwt.AlgPS512
)

// defaultAllowedAlgorithms is the verifier's fail-closed default allow-list:
// asymmetric algorithms only. Referenced by the generated options defaults.
var defaultAllowedAlgorithms = []Algorithm{AlgEdDSA, AlgES256, AlgRS256}

// DefaultAllowedAlgorithms returns a copy of the verifier's default accepted
// algorithms (EdDSA, ES256, RS256). HMAC is intentionally excluded.
func DefaultAllowedAlgorithms() []Algorithm { return jwt.DefaultAllowedAlgorithms() }
