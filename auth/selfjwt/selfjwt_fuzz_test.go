// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package selfjwt_test

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/selfjwt"
)

// FuzzVerifyRejectsUnsignedToken is the auth-bypass oracle for the self-issued
// tokens: no string a caller can assemble may verify against a key it does not
// hold.
//
// These tokens carry a tenant identity between the platform's own services, so
// Verify returning nil is what turns an arbitrary string into "this request is
// tenant X". The parser walks attacker-controlled base64url JSON to find a kid
// before it resolves any key at all, and the interesting failures live in that
// prefix — an "alg":"none" header, a kid naming another subject, a segment
// count the splitter and the verifier disagree about.
func FuzzVerifyRejectsUnsignedToken(f *testing.F) {
	f.Add("")
	f.Add("not.a.token")
	f.Add("eyJhbGciOiJub25lIn0.eyJzdWIiOiJhZG1pbiJ9.")
	f.Add("a.b.c.d")
	f.Add("...")
	f.Add("eyJhbGciOiJIUzI1NiIsImtpZCI6InRlc3Qta2V5In0.eyJzdWIiOiJhZG1pbiJ9.AAAA")

	provider := newProvider(f, selfjwt.AlgEdDSA)
	verifier := selfjwt.NewVerifier(provider)

	f.Fuzz(func(t *testing.T, raw string) {
		token, err := verifier.Verify(t.Context(), raw)
		require.Error(t, err, "Verify accepted a token nobody signed: %q", raw)
		require.Nil(t, token, "a rejected token must not also be returned")
	})
}

// FuzzMintedTokenVerifies is the same boundary from the other side: whatever the
// minter issues must verify, and must come back as the identity it was issued
// for.
//
// Subject and scopes travel as JSON inside the payload, so a subject containing
// a quote or a brace is where a minter and a verifier that disagree about
// encoding would start rejecting legitimate traffic — or, worse, return a
// subject that is not the one that was minted, which is an identity swap rather
// than an outage.
func FuzzMintedTokenVerifies(f *testing.F) {
	f.Add("read", "write")
	f.Add("", "")
	f.Add(`{"sub":"admin"}`, "*")
	f.Add("чтение", "запись")
	f.Add("a\"b", "c\\d")

	provider := newProvider(f, selfjwt.AlgEdDSA)
	minter := selfjwt.NewMinter(provider)
	verifier := selfjwt.NewVerifier(provider)

	f.Fuzz(func(t *testing.T, firstScope, secondScope string) {
		if !utf8.ValidString(firstScope) || !utf8.ValidString(secondScope) {
			t.Skip("claims are JSON; non-UTF-8 cannot survive the encoding")
		}

		res, err := minter.Mint(t.Context(), selfjwt.MintRequest{
			Subject: testSubject,
			Scopes:  []string{firstScope, secondScope},
			TTL:     time.Hour,
		})
		if err != nil {
			return // A request the minter itself refuses is not this target's business.
		}

		token, err := verifier.Verify(t.Context(), res.Token)
		require.NoError(t, err, "a freshly minted token failed its own verification")
		require.Equal(t, testSubject, token.Subject, "the subject changed on its way through the token")
		require.Equal(t, res.ID, token.ID, "the jti changed, so a revocation entry would miss it")
		require.Equal(t, []string{firstScope, secondScope}, token.Scopes)
	})
}

// FuzzVerifyRejectsTamperedToken pins what the signature is for: any edit to the
// signed portion invalidates it.
//
// The mutation lands anywhere in "header.claims" — the bytes JWT actually signs
// — so it covers the algorithm, the kid, the subject and the expiry without
// deciding in advance which matters. A verifier that resolved a key from a
// mutated header while checking the signature over the original text would pass
// the two targets above and fail here.
//
// The signature segment is out of scope on purpose: base64url without padding
// is malleable in its final character, so two spellings decode to the same
// bytes and a token mutated there verifies with nothing wrong.
func FuzzVerifyRejectsTamperedToken(f *testing.F) {
	f.Add(uint16(0), byte(1))
	f.Add(uint16(7), byte(0xff))
	f.Add(uint16(120), byte(3))

	provider := newProvider(f, selfjwt.AlgEdDSA)
	minter := selfjwt.NewMinter(provider)
	verifier := selfjwt.NewVerifier(provider)

	f.Fuzz(func(t *testing.T, position uint16, delta byte) {
		if delta == 0 {
			t.Skip("a zero delta is not a mutation")
		}

		res, err := minter.Mint(t.Context(), selfjwt.MintRequest{
			Subject: testSubject,
			TTL:     time.Hour,
		})
		require.NoError(t, err)

		signedPortion := strings.LastIndexByte(res.Token, '.')
		require.Positive(t, signedPortion, "a minted token has three segments")

		mutated := []byte(res.Token)
		at := int(position) % signedPortion
		if mutated[at] == '.' {
			t.Skip("moving a separator changes the segment count, not the content")
		}
		mutated[at] += delta
		if string(mutated) == res.Token {
			t.Skip("the mutation wrapped around to the original byte")
		}

		token, err := verifier.Verify(t.Context(), string(mutated))
		require.Error(t, err, "a token mutated at byte %d of its signed portion verified anyway", at)
		require.Nil(t, token)
	})
}
