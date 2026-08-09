// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package jwt_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/jwt"
)

// fuzzKeys mints the one key pair the targets share. Generating it per
// execution would spend the budget on ed25519 keygen instead of on the parser.
func fuzzKeys(tb testing.TB) (jwt.SigningKey, jwt.VerificationKey) {
	tb.Helper()

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(tb, err)

	return jwt.SigningKey{KeyID: "fuzz", Algorithm: jwt.AlgEdDSA, Key: priv},
		jwt.VerificationKey{Algorithm: jwt.AlgEdDSA, Key: pub}
}

// FuzzVerifyRejectsUnsignedToken is the auth-bypass oracle: no string a caller
// can assemble may verify against a key it does not hold.
//
// A bearer token is the request's entire claim to an identity, and Verify
// returning nil is what turns it into one. The parser walks three
// base64url-decoded segments of attacker-controlled JSON before it ever checks
// a signature, and the historically interesting failures live in exactly that
// prefix: an "alg":"none" header, a header that parses but a payload that does
// not, a segment count the splitter and the verifier disagree about.
func FuzzVerifyRejectsUnsignedToken(f *testing.F) {
	f.Add("")
	f.Add("not.a.token")
	f.Add("eyJhbGciOiJub25lIn0.eyJzdWIiOiJhZG1pbiJ9.") // alg=none, empty signature.
	f.Add("eyJhbGciOiJub25lIn0.eyJzdWIiOiJhZG1pbiJ9")  // Two segments only.
	f.Add("a.b.c.d")
	f.Add("...")
	f.Add("eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJhZG1pbiJ9.AAAA") // HS256 against an EdDSA key.

	f.Fuzz(func(t *testing.T, raw string) {
		_, verificationKey := fuzzKeys(t)
		verifier := jwt.NewVerifier(jwt.StaticKey(verificationKey))

		_, err := verifier.Verify(t.Context(), raw)
		require.Error(t, err,
			"Verify accepted a token that was never signed with the key: %q", raw)
	})
}

// FuzzSignedTokenVerifies is the same boundary from the other side: a token the
// signer produced must verify, whatever the claims carry.
//
// Subject and scopes travel as JSON inside the payload, so a subject containing
// a quote, a brace or a non-UTF-8 byte is where a signer and a parser that
// disagree about encoding would start rejecting legitimate traffic — the kind
// of failure that reaches production as "auth intermittently fails for some
// users" and is never reproduced from a fixture.
func FuzzSignedTokenVerifies(f *testing.F) {
	f.Add("user-1", "read")
	f.Add("", "")
	f.Add(`{"sub":"admin"}`, "write")
	f.Add("субъект", "чтение")
	f.Add("a\"b", "c\\d")
	f.Add("\x00", "\n")
	f.Add("\x84", "0") // Not UTF-8: JSON cannot carry it, see below.

	f.Fuzz(func(t *testing.T, subject, scope string) {
		if !utf8.ValidString(subject) || !utf8.ValidString(scope) {
			// Claims travel as JSON, and encoding/json substitutes U+FFFD for
			// anything that is not UTF-8. A subject that changes across the
			// round trip is the encoding's contract, not the token's — and JWT
			// claims are defined over JSON, so there is nothing else to hold.
			t.Skip("claims are JSON; non-UTF-8 cannot survive the encoding")
		}

		signingKey, verificationKey := fuzzKeys(t)

		signer := jwt.NewSigner()
		claims, err := signer.NewClaims(subject, time.Hour, scope)
		if err != nil {
			return // A subject the signer itself refuses is not this target's business.
		}

		token, err := signer.Sign(signingKey, claims)
		require.NoError(t, err)

		verified, err := jwt.NewVerifier(jwt.StaticKey(verificationKey)).Verify(t.Context(), token)
		require.NoError(t, err, "a freshly signed token failed its own verification")
		require.Equal(t, claims.Subject(), verified.Subject(),
			"the subject changed on its way through the token")
	})
}

// FuzzVerifyRejectsTamperedToken pins what a signature is for: any edit to the
// signed portion of a token invalidates it.
//
// The mutation lands anywhere in "header.claims" — the exact bytes JWT signs —
// without deciding in advance whether the algorithm, the key id, the expiry or
// the subject is the interesting one. A verifier that authenticated a mutated
// header while checking the signature over the original text would pass the two
// targets above and fail here.
//
// The signature segment itself is deliberately out of scope. Base64url without
// padding is malleable: the final character carries fewer bits than it encodes,
// so two different texts decode to the same signature bytes, and a token
// mutated there can verify without anything being wrong.
func FuzzVerifyRejectsTamperedToken(f *testing.F) {
	f.Add("user-1", uint16(0), byte(1))
	f.Add("user-1", uint16(5), byte(0xff))
	f.Add("user-1", uint16(200), byte(7))

	f.Fuzz(func(t *testing.T, subject string, position uint16, delta byte) {
		if delta == 0 {
			t.Skip("a zero delta is not a mutation")
		}
		if !utf8.ValidString(subject) {
			t.Skip("claims are JSON; non-UTF-8 cannot survive the encoding")
		}

		signingKey, verificationKey := fuzzKeys(t)

		signer := jwt.NewSigner()
		claims, err := signer.NewClaims(subject, time.Hour)
		if err != nil {
			return
		}

		token, err := signer.Sign(signingKey, claims)
		require.NoError(t, err)

		signedPortion := strings.LastIndexByte(token, '.')
		require.Positive(t, signedPortion, "a signed token has three segments")

		mutated := []byte(token)
		at := int(position) % signedPortion
		if mutated[at] == '.' {
			t.Skip("moving a separator changes the segment count, not the content")
		}
		mutated[at] += delta
		if string(mutated) == token {
			t.Skip("the mutation wrapped around to the original byte")
		}

		_, err = jwt.NewVerifier(jwt.StaticKey(verificationKey)).Verify(t.Context(), string(mutated))
		require.Error(t, err,
			"a token mutated at byte %d of its signed portion verified anyway: %q", at, string(mutated))
	})
}
