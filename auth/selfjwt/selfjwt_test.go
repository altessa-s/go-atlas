// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package selfjwt_test

import (
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/elliptic"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/selfjwt"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

const (
	testSubject = "11111111-1111-1111-1111-111111111111"
	testKeyID   = "kid-1"
	testIssuer  = "selfjwt-test"
)

// baseTime is the pinned wall clock for deterministic token timing.
var baseTime = time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)

// fixedClock is a selfjwt.Clock returning a constant instant.
type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

// clockOpt pins the minter/verifier clock to t.
func clockOpt(t time.Time) selfjwt.Option { return selfjwt.WithClock(fixedClock{t: t}) }

// keyPair is a generated signing/verification pair for one algorithm.
type keyPair struct {
	alg  selfjwt.Algorithm
	priv crypto.PrivateKey
	pub  crypto.PublicKey
}

// genKeyPair generates a fresh key pair for the given algorithm.
func genKeyPair(t testing.TB, alg selfjwt.Algorithm) keyPair {
	t.Helper()
	switch alg {
	case selfjwt.AlgEdDSA:
		pub, priv := testhelpers.GenerateEd25519Key(t)
		return keyPair{alg: alg, priv: priv, pub: pub}
	case selfjwt.AlgES256:
		k := testhelpers.GenerateECDSAKey(t, elliptic.P256())
		return keyPair{alg: alg, priv: k, pub: &k.PublicKey}
	case selfjwt.AlgRS256:
		k := testhelpers.GenerateRSAKey(t, 2048)
		return keyPair{alg: alg, priv: k, pub: &k.PublicKey}
	default:
		t.Fatalf("unsupported test algorithm %q", alg)
		return keyPair{}
	}
}

// fakeProvider is a configurable selfjwt.KeyProvider double. VerificationKey
// counts its calls so the cache can be asserted, returns selfjwt.ErrKeyRotated
// for an unknown kid and selfjwt.ErrSubjectUnknown for an unknown subject.
type fakeProvider struct {
	subject  string
	kid      string
	kp       keyPair
	vkAlg    selfjwt.Algorithm // overrides the reported verification-key algorithm when set
	pubCalls int
}

func newProvider(t testing.TB, alg selfjwt.Algorithm) *fakeProvider {
	t.Helper()
	return &fakeProvider{subject: testSubject, kid: testKeyID, kp: genKeyPair(t, alg)}
}

func (f *fakeProvider) SigningKey(_ context.Context, subject string) (selfjwt.SigningKey, error) {
	if subject != f.subject {
		return selfjwt.SigningKey{}, selfjwt.ErrSubjectUnknown
	}
	return selfjwt.SigningKey{KeyID: f.kid, Algorithm: f.kp.alg, Key: f.kp.priv}, nil
}

func (f *fakeProvider) VerificationKey(_ context.Context, subject, kid string) (selfjwt.VerificationKey, error) {
	f.pubCalls++
	if subject != f.subject {
		return selfjwt.VerificationKey{}, selfjwt.ErrSubjectUnknown
	}
	if kid != f.kid {
		return selfjwt.VerificationKey{}, selfjwt.ErrKeyRotated
	}
	alg := f.kp.alg
	if f.vkAlg != "" {
		alg = f.vkAlg
	}
	return selfjwt.VerificationKey{Algorithm: alg, Key: f.kp.pub}, nil
}

func TestRoundTrip(t *testing.T) {
	t.Parallel()
	for _, alg := range []selfjwt.Algorithm{selfjwt.AlgEdDSA, selfjwt.AlgES256, selfjwt.AlgRS256} {
		t.Run(alg.String(), func(t *testing.T) {
			t.Parallel()
			p := newProvider(t, alg)
			m := selfjwt.NewMinter(p, clockOpt(baseTime), selfjwt.WithIssuer(testIssuer))
			v := selfjwt.NewVerifier(p, clockOpt(baseTime), selfjwt.WithIssuer(testIssuer))

			res, err := m.Mint(t.Context(), selfjwt.MintRequest{Subject: testSubject, Scopes: []string{"files:read"}, TTL: time.Hour})
			require.NoError(t, err)
			require.NotEmpty(t, res.Token)
			require.Len(t, res.ID, 32) // hex of 16 random bytes
			require.Equal(t, baseTime.Add(time.Hour), res.Expiry)

			tok, err := v.Verify(t.Context(), res.Token)
			require.NoError(t, err)
			require.Equal(t, testSubject, tok.Subject)
			require.Equal(t, []string{"files:read"}, tok.Scopes)
			require.Equal(t, res.ID, tok.ID)
			require.Equal(t, baseTime.Add(time.Hour).Unix(), tok.Expiry.Unix())
		})
	}
}

func TestMintClampsTTL(t *testing.T) {
	t.Parallel()
	p := newProvider(t, selfjwt.AlgEdDSA)
	m := selfjwt.NewMinter(p, clockOpt(baseTime), selfjwt.WithMaxTokenLifetime(time.Hour))

	res, err := m.Mint(t.Context(), selfjwt.MintRequest{Subject: testSubject, TTL: 100 * 24 * time.Hour})
	require.NoError(t, err)
	require.Equal(t, baseTime.Add(time.Hour), res.Expiry)
}

func TestMintRejectsUnknownSubject(t *testing.T) {
	t.Parallel()
	p := newProvider(t, selfjwt.AlgEdDSA)
	m := selfjwt.NewMinter(p, clockOpt(baseTime))

	_, err := m.Mint(t.Context(), selfjwt.MintRequest{Subject: "nobody", TTL: time.Hour})
	require.ErrorIs(t, err, selfjwt.ErrSubjectUnknown)
}

func TestVerifyRejectsTamperedSignature(t *testing.T) {
	t.Parallel()
	p := newProvider(t, selfjwt.AlgEdDSA)
	m := selfjwt.NewMinter(p, clockOpt(baseTime))
	v := selfjwt.NewVerifier(p, clockOpt(baseTime))

	res, err := m.Mint(t.Context(), selfjwt.MintRequest{Subject: testSubject, TTL: time.Hour})
	require.NoError(t, err)

	// Flip the first character of the signature segment so the decoded signature
	// actually changes.
	dot := strings.LastIndex(res.Token, ".")
	b := []byte(res.Token)
	j := dot + 1
	if b[j] == 'A' {
		b[j] = 'B'
	} else {
		b[j] = 'A'
	}
	_, err = v.Verify(t.Context(), string(b))
	require.ErrorIs(t, err, selfjwt.ErrTokenInvalid)
}

func TestVerifyRejectsExpired(t *testing.T) {
	t.Parallel()
	p := newProvider(t, selfjwt.AlgEdDSA)
	m := selfjwt.NewMinter(p, clockOpt(baseTime))
	res, err := m.Mint(t.Context(), selfjwt.MintRequest{Subject: testSubject, TTL: time.Hour})
	require.NoError(t, err)

	late := selfjwt.NewVerifier(p, clockOpt(baseTime.Add(2*time.Hour)), selfjwt.WithLeeway(time.Minute))
	_, err = late.Verify(t.Context(), res.Token)
	require.ErrorIs(t, err, selfjwt.ErrTokenExpired)
}

func TestVerifyRejectsNotYetValid(t *testing.T) {
	t.Parallel()
	p := newProvider(t, selfjwt.AlgEdDSA)
	m := selfjwt.NewMinter(p, clockOpt(baseTime))
	res, err := m.Mint(t.Context(), selfjwt.MintRequest{Subject: testSubject, TTL: time.Hour})
	require.NoError(t, err)

	early := selfjwt.NewVerifier(p, clockOpt(baseTime.Add(-5*time.Minute)), selfjwt.WithLeeway(30*time.Second))
	_, err = early.Verify(t.Context(), res.Token)
	require.ErrorIs(t, err, selfjwt.ErrTokenInvalid)
}

func TestVerifyRejectsDisallowedAlgorithm(t *testing.T) {
	t.Parallel()
	p := newProvider(t, selfjwt.AlgEdDSA)
	v := selfjwt.NewVerifier(p, clockOpt(baseTime))

	// An HMAC token must be rejected before any key lookup: a symmetric secret
	// must never be verified as if it were a public key.
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject:   testSubject,
		ExpiresAt: jwt.NewNumericDate(baseTime.Add(time.Hour)),
	})
	tok.Header["kid"] = testKeyID
	raw, err := tok.SignedString([]byte("0123456789abcdef0123456789abcdef"))
	require.NoError(t, err)

	_, err = v.Verify(t.Context(), raw)
	require.ErrorIs(t, err, selfjwt.ErrTokenInvalid)
}

func TestVerifyHonorsAllowlistOverride(t *testing.T) {
	t.Parallel()
	p := newProvider(t, selfjwt.AlgEdDSA)
	m := selfjwt.NewMinter(p, clockOpt(baseTime))
	res, err := m.Mint(t.Context(), selfjwt.MintRequest{Subject: testSubject, TTL: time.Hour})
	require.NoError(t, err)

	// EdDSA is excluded from this verifier's allow-list, so its own minter's
	// token is rejected on algorithm grounds.
	v := selfjwt.NewVerifier(p, clockOpt(baseTime), selfjwt.WithAllowedAlgorithms(selfjwt.AlgES256))
	_, err = v.Verify(t.Context(), res.Token)
	require.ErrorIs(t, err, selfjwt.ErrTokenInvalid)
}

func TestVerifyRejectsAlgorithmMismatch(t *testing.T) {
	t.Parallel()
	p := newProvider(t, selfjwt.AlgEdDSA)
	p.vkAlg = selfjwt.AlgES256 // provider reports a different algorithm than the token carries
	m := selfjwt.NewMinter(p, clockOpt(baseTime))
	res, err := m.Mint(t.Context(), selfjwt.MintRequest{Subject: testSubject, TTL: time.Hour})
	require.NoError(t, err)

	v := selfjwt.NewVerifier(p, clockOpt(baseTime))
	_, err = v.Verify(t.Context(), res.Token)
	require.ErrorIs(t, err, selfjwt.ErrAlgorithmNotAllowed)
}

func TestVerifyRejectsMissingExpiry(t *testing.T) {
	t.Parallel()
	p := newProvider(t, selfjwt.AlgEdDSA)
	v := selfjwt.NewVerifier(p, clockOpt(baseTime))

	priv, ok := p.kp.priv.(ed25519.PrivateKey)
	require.True(t, ok)
	tok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, jwt.RegisteredClaims{
		Subject:   testSubject,
		NotBefore: jwt.NewNumericDate(baseTime),
	})
	tok.Header["kid"] = testKeyID
	raw, err := tok.SignedString(priv)
	require.NoError(t, err)

	_, err = v.Verify(t.Context(), raw)
	require.ErrorIs(t, err, selfjwt.ErrTokenInvalid)
}

func TestVerifyRejectsRotatedKid(t *testing.T) {
	t.Parallel()
	p := newProvider(t, selfjwt.AlgEdDSA)
	m := selfjwt.NewMinter(p, clockOpt(baseTime))
	res, err := m.Mint(t.Context(), selfjwt.MintRequest{Subject: testSubject, TTL: time.Hour})
	require.NoError(t, err)

	// The subject has rotated to a new kid, so the token's kid is unknown.
	rotated := &fakeProvider{subject: testSubject, kid: "kid-2", kp: p.kp}
	v := selfjwt.NewVerifier(rotated, clockOpt(baseTime))
	_, err = v.Verify(t.Context(), res.Token)
	require.ErrorIs(t, err, selfjwt.ErrKeyRotated)
}

// TestCachedKeySurvivesRotationUntilInvalidateKey documents the revocation-lag:
// once a (subject, kid) key is cached, the same verifier keeps accepting a
// token with that kid after rotation (the provider is not consulted) until the
// cache entry is dropped — here via InvalidateKey.
func TestCachedKeySurvivesRotationUntilInvalidateKey(t *testing.T) {
	t.Parallel()
	p := newProvider(t, selfjwt.AlgEdDSA)
	m := selfjwt.NewMinter(p, clockOpt(baseTime))
	v := selfjwt.NewVerifier(p, clockOpt(baseTime))

	res, err := m.Mint(t.Context(), selfjwt.MintRequest{Subject: testSubject, TTL: time.Hour})
	require.NoError(t, err)

	// Prime the cache for (subject, kid-1).
	_, err = v.Verify(t.Context(), res.Token)
	require.NoError(t, err)
	require.Equal(t, 1, p.pubCalls)

	// Rotate: the provider now only knows kid-2 and would reject kid-1.
	p.kid = "kid-2"

	// Within the cache TTL the rotated-away token still verifies from cache; the
	// provider is not consulted, so ErrKeyRotated is never observed.
	_, err = v.Verify(t.Context(), res.Token)
	require.NoError(t, err)
	require.Equal(t, 1, p.pubCalls)

	// InvalidateKey drops the entry, forcing a provider lookup that now rejects.
	v.InvalidateKey(testSubject, testKeyID)
	_, err = v.Verify(t.Context(), res.Token)
	require.ErrorIs(t, err, selfjwt.ErrKeyRotated)
	require.Equal(t, 2, p.pubCalls)
}

// TestInvalidateSubjectClosesRotationWindow verifies InvalidateSubject drops a
// subject's cached key without the caller knowing the retired kid.
func TestInvalidateSubjectClosesRotationWindow(t *testing.T) {
	t.Parallel()
	p := newProvider(t, selfjwt.AlgEdDSA)
	m := selfjwt.NewMinter(p, clockOpt(baseTime))
	v := selfjwt.NewVerifier(p, clockOpt(baseTime))

	res, err := m.Mint(t.Context(), selfjwt.MintRequest{Subject: testSubject, TTL: time.Hour})
	require.NoError(t, err)

	_, err = v.Verify(t.Context(), res.Token)
	require.NoError(t, err)

	p.kid = "kid-2"
	v.InvalidateSubject(testSubject)

	_, err = v.Verify(t.Context(), res.Token)
	require.ErrorIs(t, err, selfjwt.ErrKeyRotated)
}

// TestCacheTTLZeroDisablesCaching verifies WithCacheTTL(0) bypasses the cache so
// every verification consults the KeyProvider (no rotation lag).
func TestCacheTTLZeroDisablesCaching(t *testing.T) {
	t.Parallel()
	p := newProvider(t, selfjwt.AlgEdDSA)
	m := selfjwt.NewMinter(p, clockOpt(baseTime))
	v := selfjwt.NewVerifier(p, clockOpt(baseTime), selfjwt.WithCacheTTL(0))

	res, err := m.Mint(t.Context(), selfjwt.MintRequest{Subject: testSubject, TTL: time.Hour})
	require.NoError(t, err)

	_, err = v.Verify(t.Context(), res.Token)
	require.NoError(t, err)
	_, err = v.Verify(t.Context(), res.Token)
	require.NoError(t, err)
	require.Equal(t, 2, p.pubCalls) // no caching: provider hit on every verify
}

func TestVerifyRejectsUnknownSubject(t *testing.T) {
	t.Parallel()
	p := newProvider(t, selfjwt.AlgEdDSA)
	m := selfjwt.NewMinter(p, clockOpt(baseTime))
	res, err := m.Mint(t.Context(), selfjwt.MintRequest{Subject: testSubject, TTL: time.Hour})
	require.NoError(t, err)

	other := &fakeProvider{subject: "22222222-2222-2222-2222-222222222222", kid: testKeyID, kp: p.kp}
	v := selfjwt.NewVerifier(other, clockOpt(baseTime))
	_, err = v.Verify(t.Context(), res.Token)
	require.ErrorIs(t, err, selfjwt.ErrSubjectUnknown)
}

func TestVerifyCachesVerificationKey(t *testing.T) {
	t.Parallel()
	p := newProvider(t, selfjwt.AlgEdDSA)
	m := selfjwt.NewMinter(p, clockOpt(baseTime))
	res, err := m.Mint(t.Context(), selfjwt.MintRequest{Subject: testSubject, TTL: time.Hour})
	require.NoError(t, err)

	v := selfjwt.NewVerifier(p, clockOpt(baseTime))
	_, err = v.Verify(t.Context(), res.Token)
	require.NoError(t, err)
	_, err = v.Verify(t.Context(), res.Token)
	require.NoError(t, err)
	require.Equal(t, 1, p.pubCalls) // second verify served from cache
}

func TestNewPairsMinterAndVerifier(t *testing.T) {
	t.Parallel()
	p := newProvider(t, selfjwt.AlgEdDSA)
	// One option list builds both sides, so the issuer (and clock) cannot drift.
	m, v := selfjwt.New(p, clockOpt(baseTime), selfjwt.WithIssuer(testIssuer))

	res, err := m.Mint(t.Context(), selfjwt.MintRequest{Subject: testSubject, TTL: time.Hour})
	require.NoError(t, err)
	tok, err := v.Verify(t.Context(), res.Token)
	require.NoError(t, err)
	require.Equal(t, testSubject, tok.Subject)
}

func TestMintRejectsEmptySubject(t *testing.T) {
	t.Parallel()
	p := newProvider(t, selfjwt.AlgEdDSA)
	m := selfjwt.NewMinter(p, clockOpt(baseTime))

	_, err := m.Mint(t.Context(), selfjwt.MintRequest{TTL: time.Hour})
	require.ErrorIs(t, err, selfjwt.ErrSubjectRequired)
}

func TestMintRejectsEmptyKeyID(t *testing.T) {
	t.Parallel()
	p := newProvider(t, selfjwt.AlgEdDSA)
	p.kid = "" // provider hands back signing material with no key id
	m := selfjwt.NewMinter(p, clockOpt(baseTime))

	_, err := m.Mint(t.Context(), selfjwt.MintRequest{Subject: testSubject, TTL: time.Hour})
	require.ErrorIs(t, err, selfjwt.ErrSigningKeyInvalid)
}

// noAlgProvider wraps a provider but strips the algorithm off every resolved
// verification key, exercising the verifier's requirement that a key name the
// algorithm it is bound to.
type noAlgProvider struct{ inner selfjwt.KeyProvider }

func (p noAlgProvider) SigningKey(ctx context.Context, subject string) (selfjwt.SigningKey, error) {
	return p.inner.SigningKey(ctx, subject)
}

func (p noAlgProvider) VerificationKey(ctx context.Context, subject, kid string) (selfjwt.VerificationKey, error) {
	vk, err := p.inner.VerificationKey(ctx, subject, kid)
	if err != nil {
		return selfjwt.VerificationKey{}, err
	}
	vk.Algorithm = ""
	return vk, nil
}

func TestVerifyRejectsKeyWithoutAlgorithm(t *testing.T) {
	t.Parallel()
	p := newProvider(t, selfjwt.AlgEdDSA)
	m := selfjwt.NewMinter(p, clockOpt(baseTime))
	res, err := m.Mint(t.Context(), selfjwt.MintRequest{Subject: testSubject, TTL: time.Hour})
	require.NoError(t, err)

	v := selfjwt.NewVerifier(noAlgProvider{inner: p}, clockOpt(baseTime))
	_, err = v.Verify(t.Context(), res.Token)
	require.ErrorIs(t, err, selfjwt.ErrAlgorithmNotAllowed)
}
