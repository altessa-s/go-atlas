// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package keyset_test

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/keyset"
)

var (
	key      = bytes.Repeat([]byte{1}, keyset.MinKeyLength)
	otherKey = bytes.Repeat([]byte{2}, keyset.MinKeyLength)
	bindings = keyset.Bindings{Sort: "ts:desc", Filter: "actor=a", Subject: "user-1"}
)

func fixedClock(t time.Time) keyset.Clock { return func() time.Time { return t } }

func newCodec(t *testing.T, opts ...keyset.Option) *keyset.Codec {
	t.Helper()
	c, err := keyset.New(key, opts...)
	require.NoError(t, err)
	return c
}

func TestNew_RejectsWeakKeys(t *testing.T) {
	t.Parallel()

	_, err := keyset.New([]byte("short"))
	require.ErrorIs(t, err, keyset.ErrWeakKey)

	_, err = keyset.New(key, keyset.WithPreviousKeys([]byte("short")))
	require.ErrorIs(t, err, keyset.ErrWeakKey)
}

func TestCodec_RoundTrip(t *testing.T) {
	t.Parallel()

	c := newCodec(t)
	token, err := c.Issue([]byte("1700000000000|01J"), bindings)
	require.NoError(t, err)

	payload, err := c.Resolve(token, bindings)
	require.NoError(t, err)
	require.Equal(t, []byte("1700000000000|01J"), payload)
}

func TestCodec_RejectsTamperingAndForeignKeys(t *testing.T) {
	t.Parallel()

	c := newCodec(t)
	token, err := c.Issue([]byte("payload"), bindings)
	require.NoError(t, err)

	raw, err := base64.RawURLEncoding.DecodeString(token)
	require.NoError(t, err)
	for i := range raw {
		tampered := bytes.Clone(raw)
		tampered[i] ^= 0x01
		_, err := c.Resolve(base64.RawURLEncoding.EncodeToString(tampered), bindings)
		require.ErrorIs(t, err, keyset.ErrInvalidToken, "flipping byte %d must invalidate the token", i)
	}

	foreign, err := keyset.New(otherKey)
	require.NoError(t, err)
	_, err = foreign.Resolve(token, bindings)
	require.ErrorIs(t, err, keyset.ErrInvalidToken)
}

// A forged token reports ErrInvalidToken even when its bindings differ: the
// signature is checked before the bindings.
func TestCodec_SignatureCheckedBeforeBindings(t *testing.T) {
	t.Parallel()

	foreign, err := keyset.New(otherKey)
	require.NoError(t, err)
	token, err := foreign.Issue([]byte("p"), keyset.Bindings{Sort: "other"})
	require.NoError(t, err)

	_, err = newCodec(t).Resolve(token, bindings)
	require.ErrorIs(t, err, keyset.ErrInvalidToken)
}

func TestCodec_KeyRotation(t *testing.T) {
	t.Parallel()

	old, err := keyset.New(otherKey)
	require.NoError(t, err)
	token, err := old.Issue([]byte("p"), bindings)
	require.NoError(t, err)

	rotated := newCodec(t, keyset.WithPreviousKeys(otherKey))
	payload, err := rotated.Resolve(token, bindings)
	require.NoError(t, err)
	require.Equal(t, []byte("p"), payload)
}

func TestCodec_Bindings(t *testing.T) {
	t.Parallel()

	c := newCodec(t)
	token, err := c.Issue([]byte("p"), bindings)
	require.NoError(t, err)

	for name, tc := range map[string]struct {
		b    keyset.Bindings
		want error
	}{
		"sort":    {keyset.Bindings{Sort: "ts:asc", Filter: bindings.Filter, Subject: bindings.Subject}, keyset.ErrSortChanged},
		"filter":  {keyset.Bindings{Sort: bindings.Sort, Filter: "actor=b", Subject: bindings.Subject}, keyset.ErrFilterChanged},
		"subject": {keyset.Bindings{Sort: bindings.Sort, Filter: bindings.Filter, Subject: "user-2"}, keyset.ErrSubjectMismatch},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := c.Resolve(token, tc.b)
			require.ErrorIs(t, err, tc.want)
		})
	}
}

func TestCodec_Expiry(t *testing.T) {
	t.Parallel()

	issuedAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	token, err := newCodec(t, keyset.WithClock(fixedClock(issuedAt))).Issue([]byte("p"), bindings)
	require.NoError(t, err)

	_, err = newCodec(t, keyset.WithClock(fixedClock(issuedAt.Add(keyset.DefaultTTL-time.Second)))).Resolve(token, bindings)
	require.NoError(t, err)

	_, err = newCodec(t, keyset.WithClock(fixedClock(issuedAt.Add(keyset.DefaultTTL+time.Second)))).Resolve(token, bindings)
	require.ErrorIs(t, err, keyset.ErrExpiredToken)

	_, err = newCodec(t, keyset.WithExpiryDisabled(), keyset.WithClock(fixedClock(issuedAt.Add(365*24*time.Hour)))).Resolve(token, bindings)
	require.NoError(t, err, "WithExpiryDisabled disables expiry")

	_, err = newCodec(t, keyset.WithClock(fixedClock(issuedAt.Add(-time.Hour)))).Resolve(token, bindings)
	require.ErrorIs(t, err, keyset.ErrInvalidToken, "a token issued in the future beyond the skew is rejected")
}

func TestCodec_Bounds(t *testing.T) {
	t.Parallel()

	c := newCodec(t)
	_, err := c.Issue(make([]byte, keyset.MaxPayloadLength+1), bindings)
	require.ErrorIs(t, err, keyset.ErrPayloadTooLarge)

	_, err = c.Issue(make([]byte, keyset.MaxPayloadLength), bindings)
	require.NoError(t, err)

	for _, token := range []string{"", "!!!", strings.Repeat("A", 1<<16)} {
		_, err := c.Resolve(token, bindings)
		require.ErrorIs(t, err, keyset.ErrInvalidToken)
	}
}

// Changing the caller's key buffers after New does not change which tokens
// verify.
func TestNew_CopiesKeys(t *testing.T) {
	t.Parallel()

	current, previous := bytes.Clone(key), bytes.Clone(otherKey)
	c, err := keyset.New(current, keyset.WithPreviousKeys(previous))
	require.NoError(t, err)

	old, err := keyset.New(otherKey)
	require.NoError(t, err)
	token, err := old.Issue([]byte("p"), bindings)
	require.NoError(t, err)

	clear(current)
	clear(previous)
	_, err = c.Resolve(token, bindings)
	require.NoError(t, err)
}
