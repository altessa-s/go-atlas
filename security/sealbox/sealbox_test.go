// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sealbox_test

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/security/sealbox"
)

func newKey(t testing.TB) string {
	t.Helper()
	k := make([]byte, sealbox.KeyLen)
	_, err := rand.Read(k)
	require.NoError(t, err)
	return base64.StdEncoding.EncodeToString(k)
}

func unhex(t testing.TB, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	require.NoError(t, err)
	return b
}

func TestSealOpenRoundTrip(t *testing.T) {
	t.Parallel()

	box, err := sealbox.New(newKey(t))
	require.NoError(t, err)
	aad := []byte("provider:openai")
	sealed, err := box.Seal([]byte("sk-secret"), aad)
	require.NoError(t, err)
	require.True(t, sealed.Set())
	require.Len(t, sealed.KeyID, sealbox.KeyIDLen)
	require.Len(t, sealed.Nonce, 12)

	got, err := box.Open(sealed, aad)
	require.NoError(t, err)
	require.Equal(t, "sk-secret", string(got))

	// Wrong associated data must not decrypt.
	_, err = box.Open(sealed, []byte("provider:other"))
	require.ErrorIs(t, err, sealbox.ErrOpen)
}

func TestSealUsesFreshNonces(t *testing.T) {
	t.Parallel()

	box, err := sealbox.New(newKey(t))
	require.NoError(t, err)
	a, err := box.Seal([]byte("same"), nil)
	require.NoError(t, err)
	b, err := box.Seal([]byte("same"), nil)
	require.NoError(t, err)
	require.NotEqual(t, a.Nonce, b.Nonce)
	require.NotEqual(t, a.Ciphertext, b.Ciphertext)
}

func TestNilBoxHasNoKey(t *testing.T) {
	t.Parallel()

	for _, key := range []string{"", "  \n"} {
		box, err := sealbox.New(key)
		require.NoError(t, err)
		require.Nil(t, box, "an empty key yields a nil box")

		_, err = box.Seal([]byte("x"), nil)
		require.ErrorIs(t, err, sealbox.ErrNoKey)
		_, err = box.Open(sealbox.Sealed{KeyID: "k", Nonce: make([]byte, 12), Ciphertext: []byte("c")}, nil)
		require.ErrorIs(t, err, sealbox.ErrNoKey)
		require.False(t, box.Current(sealbox.Sealed{}))
		require.False(t, box.Opens(sealbox.Sealed{}))
	}
}

func TestRotationOpensPrevious(t *testing.T) {
	t.Parallel()

	oldKey := newKey(t)
	old, err := sealbox.New(oldKey)
	require.NoError(t, err)
	sealed, err := old.Seal([]byte("k"), nil)
	require.NoError(t, err)
	require.True(t, old.Current(sealed))

	// New box with a fresh current key and the old one as previous.
	rotated, err := sealbox.New(newKey(t), oldKey)
	require.NoError(t, err)
	got, err := rotated.Open(sealed, nil)
	require.NoError(t, err)
	require.Equal(t, "k", string(got))
	require.False(t, rotated.Current(sealed), "a value sealed with the previous key must not report as current")
	require.True(t, rotated.Opens(sealed))

	resealed, err := rotated.Seal(got, nil)
	require.NoError(t, err)
	require.True(t, rotated.Current(resealed))
}

func TestUnknownKey(t *testing.T) {
	t.Parallel()

	a, err := sealbox.New(newKey(t))
	require.NoError(t, err)
	b, err := sealbox.New(newKey(t))
	require.NoError(t, err)
	sealed, err := a.Seal([]byte("v"), nil)
	require.NoError(t, err)

	_, err = b.Open(sealed, nil)
	require.ErrorIs(t, err, sealbox.ErrUnknownKey)
	require.False(t, b.Opens(sealed))
}

func TestOpenRejectsTampering(t *testing.T) {
	t.Parallel()

	box, err := sealbox.New(newKey(t))
	require.NoError(t, err)
	sealed, err := box.Seal([]byte("value"), []byte("aad"))
	require.NoError(t, err)

	flipped := sealed
	flipped.Ciphertext = append([]byte(nil), sealed.Ciphertext...)
	flipped.Ciphertext[0] ^= 0x01

	shortNonce := sealed
	shortNonce.Nonce = sealed.Nonce[:8]

	otherNonce := sealed
	otherNonce.Nonce = append([]byte(nil), sealed.Nonce...)
	otherNonce.Nonce[0] ^= 0x01

	for name, s := range map[string]sealbox.Sealed{
		"ciphertext": flipped,
		"short":      shortNonce,
		"nonce":      otherNonce,
		"truncated":  {KeyID: sealed.KeyID, Nonce: sealed.Nonce, Ciphertext: sealed.Ciphertext[:4]},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := box.Open(s, []byte("aad"))
			require.ErrorIs(t, err, sealbox.ErrOpen)
		})
	}
}

func TestNewRejectsMalformedKeys(t *testing.T) {
	t.Parallel()

	short := base64.StdEncoding.EncodeToString(make([]byte, 16))
	for name, keys := range map[string][]string{
		"current not base64":  {"not base64!"},
		"current short":       {short},
		"previous not base64": {newKey(t), "%%%secret%%%"},
		"previous short":      {newKey(t), short},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			box, err := sealbox.New(keys[0], keys[1:]...)
			require.Error(t, err)
			require.Nil(t, box)
			for _, k := range keys {
				require.NotContains(t, err.Error(), k, "the error must not quote a key")
			}
		})
	}
}

func TestKeyWhitespaceIgnored(t *testing.T) {
	t.Parallel()

	key := newKey(t)
	plain, err := sealbox.New(key)
	require.NoError(t, err)
	padded, err := sealbox.New("  " + key + "\n")
	require.NoError(t, err)
	sealed, err := plain.Seal([]byte("v"), nil)
	require.NoError(t, err)
	require.True(t, padded.Current(sealed))
}

func TestKeyID(t *testing.T) {
	t.Parallel()

	key := make([]byte, sealbox.KeyLen)
	for i := range key {
		key[i] = byte(i)
	}
	require.Equal(t, "630dcd2966c43366", sealbox.KeyID(key))
}

// TestGoldenVectors opens ciphertexts produced by the code this package was moved from (the radar service), so values
// already stored (e.g. AI provider API keys in MongoDB) keep decrypting: key ID = hex(sha256(key))[:16], 12-byte random nonce, GCM
// ciphertext with the tag appended.
func TestGoldenVectors(t *testing.T) {
	t.Parallel()

	const (
		current  = "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=" // bytes 0x00..0x1f
		previous = "//79/Pv6+fj39vX08/Lx8O/u7ezr6uno5+bl5OPi4eA=" // bytes 0xff..0xe0
	)
	box, err := sealbox.New(current, previous)
	require.NoError(t, err)

	cases := []struct {
		name    string
		sealed  sealbox.Sealed
		aad     []byte
		want    string
		current bool
	}{
		{
			name: "current key with aad",
			sealed: sealbox.Sealed{
				KeyID:      "630dcd2966c43366",
				Nonce:      unhex(t, "430b39daeaf0aafd1b946a41"),
				Ciphertext: unhex(t, "aade320b8bb1abc16b4c781f5cce89e85c3fbb6ff7688f34bbeb08bd471acc74"),
			},
			aad:     []byte("ai-provider:p1|openai"),
			want:    "sk-golden-secret",
			current: true,
		},
		{
			name: "previous key without aad",
			sealed: sealbox.Sealed{
				KeyID:      "1865c00831e73f7e",
				Nonce:      unhex(t, "5f1bfb98c1e65bfe3fd252e4"),
				Ciphertext: unhex(t, "99a384c240631fba067c80ab27e76a85c751dd88c30636276c48a4"),
			},
			want: "sk-previous",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := box.Open(tc.sealed, tc.aad)
			require.NoError(t, err)
			require.Equal(t, tc.want, string(got))
			require.Equal(t, tc.current, box.Current(tc.sealed))
		})
	}
}

func TestConcurrentUse(t *testing.T) {
	t.Parallel()

	box, err := sealbox.New(newKey(t), newKey(t))
	require.NoError(t, err)
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			for range 50 {
				s, err := box.Seal([]byte("v"), []byte("a"))
				if err != nil {
					t.Error(err)
					return
				}
				if _, err := box.Open(s, []byte("a")); err != nil {
					t.Error(err)
					return
				}
			}
		})
	}
	wg.Wait()
}
