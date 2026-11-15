// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package pagingconfig_test

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-nomos/loader"
	"github.com/altessa-s/go-nomos/loader/backend/yaml3"

	"github.com/altessa-s/go-atlas-core/types/redacted"

	pagingconfig "github.com/altessa-s/go-atlas/config/paging"
)

// raw returns a raw key of n bytes.
func raw(n int) redacted.RedactedString {
	return redacted.RedactedString(strings.Repeat("k", n))
}

// b64 returns a base64 key decoding to n bytes.
func b64(n int) redacted.RedactedString {
	return redacted.RedactedString(base64.StdEncoding.EncodeToString([]byte(strings.Repeat("\xff", n))))
}

func TestConfig_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     pagingconfig.Config
		wantErr bool
	}{
		{"raw key", pagingconfig.Config{SigningKey: raw(32)}, false},
		{"explicit raw", pagingconfig.Config{SigningKey: raw(32), KeyEncoding: pagingconfig.KeyEncodingRaw}, false},
		{"missing key", pagingconfig.Config{}, true},
		{"raw key one byte short", pagingconfig.Config{SigningKey: raw(31)}, true},
		{"raw multibyte runes count bytes", pagingconfig.Config{SigningKey: redacted.RedactedString(strings.Repeat("я", 16))}, false},
		{"base64 key", pagingconfig.Config{SigningKey: b64(32), KeyEncoding: pagingconfig.KeyEncodingBase64}, false},
		{"base64 key one byte short", pagingconfig.Config{SigningKey: b64(31), KeyEncoding: pagingconfig.KeyEncodingBase64}, true},
		{"not base64", pagingconfig.Config{SigningKey: "!!!!" + raw(44), KeyEncoding: pagingconfig.KeyEncodingBase64}, true},
		{"unknown encoding", pagingconfig.Config{SigningKey: raw(32), KeyEncoding: "hex"}, true},
		{"empty previous key", pagingconfig.Config{SigningKey: raw(32), PreviousKeys: []redacted.RedactedString{""}}, true},
		{"short previous key", pagingconfig.Config{SigningKey: raw(32), PreviousKeys: []redacted.RedactedString{raw(32), raw(31)}}, true},
		{
			"base64 previous key",
			pagingconfig.Config{SigningKey: b64(32), PreviousKeys: []redacted.RedactedString{b64(40)}, KeyEncoding: pagingconfig.KeyEncodingBase64},
			false,
		},
		{"zero ttl keeps default", pagingconfig.Config{SigningKey: raw(32)}, false},
		{"negative ttl", pagingconfig.Config{SigningKey: raw(32), TokenTTL: -time.Second}, true},
		{"encryption", pagingconfig.Config{SigningKey: raw(32), Encryption: pagingconfig.Encryption{Enabled: true}}, false},
		{"strict encryption", pagingconfig.Config{SigningKey: raw(32), Encryption: pagingconfig.Encryption{Enabled: true, Strict: true}}, false},
		{"strict without encryption", pagingconfig.Config{SigningKey: raw(32), Encryption: pagingconfig.Encryption{Strict: true}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.cfg.Validate()
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestConfig_ValidateNeverQuotesKey(t *testing.T) {
	t.Parallel()

	key := "not-base64-but-a-secret-of-more-than-32-bytes!"
	cfg := pagingconfig.Config{SigningKey: redacted.RedactedString(key), KeyEncoding: pagingconfig.KeyEncodingBase64}
	err := cfg.Validate()
	require.Error(t, err)
	require.NotContains(t, err.Error(), key)
}

func TestConfig_Keys(t *testing.T) {
	t.Parallel()

	t.Run("base64", func(t *testing.T) {
		t.Parallel()
		cfg := pagingconfig.Config{
			SigningKey:   b64(32),
			PreviousKeys: []redacted.RedactedString{b64(33)},
			KeyEncoding:  pagingconfig.KeyEncodingBase64,
		}
		current, previous, err := cfg.Keys()
		require.NoError(t, err)
		require.Equal(t, []byte(strings.Repeat("\xff", 32)), current)
		require.Equal(t, [][]byte{[]byte(strings.Repeat("\xff", 33))}, previous)
	})
	t.Run("raw", func(t *testing.T) {
		t.Parallel()
		cfg := pagingconfig.Config{SigningKey: raw(32)}
		current, previous, err := cfg.Keys()
		require.NoError(t, err)
		require.Equal(t, []byte(raw(32).Expose()), current)
		require.Empty(t, previous)
	})
	t.Run("bad previous key", func(t *testing.T) {
		t.Parallel()
		cfg := pagingconfig.Config{
			SigningKey:   b64(32),
			PreviousKeys: []redacted.RedactedString{"%%%"},
			KeyEncoding:  pagingconfig.KeyEncodingBase64,
		}
		_, _, err := cfg.Keys()
		require.ErrorIs(t, err, pagingconfig.ErrKeyEncoding)
	})
}

func TestDecodeKey(t *testing.T) {
	t.Parallel()

	_, err := pagingconfig.DecodeKey(pagingconfig.KeyEncodingBase64, "%%%")
	require.ErrorIs(t, err, pagingconfig.ErrKeyEncoding)
	_, err = pagingconfig.DecodeKey("hex", raw(32))
	require.ErrorIs(t, err, pagingconfig.ErrKeyEncoding)
	got, err := pagingconfig.DecodeKey("", raw(3))
	require.NoError(t, err, "length is checked by Validate and pagetoken.New, not by decoding")
	require.Equal(t, []byte("kkk"), got)

	// Line breaks inside a base64 value are ignored, as a mounted file may
	// wrap it.
	wrapped := redacted.RedactedString(string(b64(32)[:20]) + "\n" + string(b64(32)[20:]) + "\n")
	got, err = pagingconfig.DecodeKey(pagingconfig.KeyEncodingBase64, wrapped)
	require.NoError(t, err)
	require.Len(t, got, 32)
}

func TestKeyRule(t *testing.T) {
	t.Parallel()

	rule := pagingconfig.KeyRule(pagingconfig.KeyEncodingBase64)
	require.NoError(t, rule.Validate(redacted.RedactedString("")), "an empty key is left to Required")
	require.NoError(t, rule.Validate(b64(32)))
	require.ErrorIs(t, rule.Validate(b64(8)), pagingconfig.ErrKeyTooShort)
	require.Error(t, rule.Validate("plain string"))
}

// TestConfig_LoaderDefaults is serial: it sets process environment variables.
func TestConfig_LoaderDefaults(t *testing.T) {
	type root struct {
		Paging *pagingconfig.Config `yaml:"paging"`
	}
	t.Setenv("PAGINGTEST_PAGING__SIGNING_KEY", string(b64(32)))
	t.Setenv("PAGINGTEST_PAGING__KEY_ENCODING", "base64")
	t.Setenv("PAGINGTEST_PAGING__ENCRYPTION__ENABLED", "true")
	loaded, err := loader.New(&yaml3.Backend{}, loader.WithEnvPrefix("PAGINGTEST")).Load(&root{})
	require.NoError(t, err)
	cfg := loaded.(*root).Paging //nolint:forcetypeassert // Load returns the pointer it was given
	require.Equal(t, pagingconfig.DefaultTokenTTL, cfg.TokenTTL)
	require.Equal(t, pagingconfig.KeyEncodingBase64, cfg.KeyEncoding)
	require.Equal(t, pagingconfig.Encryption{Enabled: true}, cfg.Encryption)
	require.NoError(t, cfg.Validate())
}
