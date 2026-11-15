// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-aip/pagination/pagetoken"

	"github.com/altessa-s/go-atlas-core/types/redacted"
	"github.com/altessa-s/go-atlas/factory/ext/aip/pagination/pagetoken"

	pagingconfig "github.com/altessa-s/go-atlas/config/paging"
)

func TestBuild_NilConfig(t *testing.T) {
	t.Parallel()

	_, err := factory.New(nil).Build()
	require.Error(t, err)
}

func TestBuild_InvalidKey(t *testing.T) {
	t.Parallel()

	_, err := factory.New(&pagingconfig.Config{SigningKey: "short"}).Build()
	require.ErrorIs(t, err, pagetoken.ErrWeakKey)
}

// TestBuild_Base64Rotation signs a token with a base64 key and resolves it
// after a rotation that keeps the old key as a previous key: the configured
// encoding is applied to every key.
func TestBuild_Base64Rotation(t *testing.T) {
	t.Parallel()

	encode := func(b byte) redacted.RedactedString {
		return redacted.RedactedString(base64.StdEncoding.EncodeToString([]byte(strings.Repeat(string(b), 32))))
	}
	bindings := pagetoken.NewBindings("list", "")
	old, err := factory.New(&pagingconfig.Config{SigningKey: encode('a'), KeyEncoding: pagingconfig.KeyEncodingBase64}).Build()
	require.NoError(t, err)
	token, err := pagetoken.NewTyped[int](old).Issue(7, bindings)
	require.NoError(t, err)

	rotated, err := factory.New(&pagingconfig.Config{
		SigningKey:   encode('b'),
		PreviousKeys: []redacted.RedactedString{encode('a')},
		KeyEncoding:  pagingconfig.KeyEncodingBase64,
	}).Build()
	require.NoError(t, err)
	got, _, err := pagetoken.NewTyped[int](rotated).Resolve(token, bindings)
	require.NoError(t, err)
	require.Equal(t, 7, got)

	// The raw string of the same base64 text is a different key.
	asRaw, err := factory.New(&pagingconfig.Config{SigningKey: encode('a')}).Build()
	require.NoError(t, err)
	_, _, err = pagetoken.NewTyped[int](asRaw).Resolve(token, bindings)
	require.Error(t, err)
}

func TestBuild_TokenTTL(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	key := redacted.RedactedString(strings.Repeat("k", 32))
	bindings := pagetoken.NewBindings("list", "")

	codec, err := factory.New(&pagingconfig.Config{SigningKey: key, TokenTTL: time.Minute}).UseOptions(pagetoken.WithClock(clock)).Build()
	require.NoError(t, err)
	token, err := pagetoken.NewTyped[int](codec).Issue(1, bindings)
	require.NoError(t, err)

	later, err := factory.New(&pagingconfig.Config{SigningKey: key, TokenTTL: time.Minute}).
		UseOptions(pagetoken.WithClock(func() time.Time { return now.Add(2 * time.Minute) })).Build()
	require.NoError(t, err)
	_, _, err = pagetoken.NewTyped[int](later).Resolve(token, bindings)
	require.Error(t, err, "a token older than tokenTTL expires")

	// Zero keeps the default lifetime (24h).
	defaulted, err := factory.New(&pagingconfig.Config{SigningKey: key}).
		UseOptions(pagetoken.WithClock(func() time.Time { return now.Add(2 * time.Minute) })).Build()
	require.NoError(t, err)
	_, _, err = pagetoken.NewTyped[int](defaulted).Resolve(token, bindings)
	require.NoError(t, err)
}

// TestBuild_Encryption maps the encryption block: enabled issues encrypted
// tokens, which a strict codec resolves, while strict rejects the signed
// tokens of a codec without encryption.
func TestBuild_Encryption(t *testing.T) {
	t.Parallel()

	key := redacted.RedactedString(strings.Repeat("k", 32))
	bindings := pagetoken.NewBindings("list", "")
	build := func(enc pagingconfig.Encryption) pagetoken.Typed[int] {
		t.Helper()
		codec, err := factory.New(&pagingconfig.Config{SigningKey: key, Encryption: enc}).Build()
		require.NoError(t, err)
		return pagetoken.NewTyped[int](codec)
	}
	strict := build(pagingconfig.Encryption{Enabled: true, Strict: true})

	encrypted, err := build(pagingconfig.Encryption{Enabled: true}).Issue(3, bindings)
	require.NoError(t, err)
	got, _, err := strict.Resolve(encrypted, bindings)
	require.NoError(t, err, "an enabled codec issues encrypted tokens")
	require.Equal(t, 3, got)

	signed, err := build(pagingconfig.Encryption{}).Issue(3, bindings)
	require.NoError(t, err)
	_, _, err = strict.Resolve(signed, bindings)
	require.ErrorIs(t, err, pagetoken.ErrUnencryptedToken)

	strictToken, err := strict.Issue(4, bindings)
	require.NoError(t, err)
	got, _, err = build(pagingconfig.Encryption{}).Resolve(strictToken, bindings)
	require.NoError(t, err, "every codec opens encrypted tokens")
	require.Equal(t, 4, got)
}
