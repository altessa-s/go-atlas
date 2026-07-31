// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	corehash "github.com/altessa-s/go-atlas/core/encoding/hash"
)

func TestParseKeyLogMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want KeyLogMode
	}{
		{"hashed", "hashed", KeyLogHashed},
		{"full", "full", KeyLogFull},
		{"off", "off", KeyLogOff},
		{"empty_falls_back_to_hashed", "", KeyLogHashed},
		{"unknown_falls_back_to_hashed", "verbose", KeyLogHashed},
		{"case_sensitive_falls_back_to_hashed", "Full", KeyLogHashed},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, ParseKeyLogMode(tc.in))
		})
	}
}

func TestKeyLogMode_IsValid(t *testing.T) {
	t.Parallel()

	for _, mode := range []KeyLogMode{KeyLogHashed, KeyLogFull, KeyLogOff} {
		require.True(t, mode.IsValid(), "%q should be valid", mode)
		require.Equal(t, string(mode), mode.String())
	}

	require.False(t, KeyLogMode("").IsValid())
	require.False(t, KeyLogMode("verbose").IsValid())
}

func TestMiddleware_KeyLogAttrs(t *testing.T) {
	t.Parallel()

	const key = "550e8400-e29b-41d4-a716-446655440000"
	wantHash := corehash.SHA256HexString(key)[:keyLogHashLength]

	tests := []struct {
		name     string
		mode     KeyLogMode
		wantKeys []string
		wantVals []string
	}{
		{
			name:     "hashed_logs_digest",
			mode:     KeyLogHashed,
			wantKeys: []string{"method", "key_hash"},
			wantVals: []string{http.MethodPost, wantHash},
		},
		{
			name:     "full_logs_raw_key",
			mode:     KeyLogFull,
			wantKeys: []string{"method", "key"},
			wantVals: []string{http.MethodPost, key},
		},
		{
			name:     "off_omits_key",
			mode:     KeyLogOff,
			wantKeys: []string{"method"},
			wantVals: []string{http.MethodPost},
		},
		{
			name:     "unset_mode_falls_back_to_hashed",
			mode:     KeyLogMode(""),
			wantKeys: []string{"method", "key_hash"},
			wantVals: []string{http.MethodPost, wantHash},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m := New(nil, WithLogger(debugLogger(io.Discard)), WithKeyLogMode(tc.mode))
			attrs := m.keyLogAttrs(t.Context(), http.MethodPost, key)

			require.Len(t, attrs, len(tc.wantKeys))
			for i, attr := range attrs {
				require.Equal(t, tc.wantKeys[i], attr.Key)
				require.Equal(t, tc.wantVals[i], attr.Value.String())
			}
		})
	}
}

// TestMiddleware_KeyLogAttrs_SkippedWhenDebugDisabled pins that the digest is
// not computed on the request path of a production logger at Info or above.
func TestMiddleware_KeyLogAttrs_SkippedWhenDebugDisabled(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelInfo}))
	m := New(nil, WithLogger(logger))

	require.Nil(t, m.keyLogAttrs(t.Context(), http.MethodPost, "550e8400-e29b-41d4-a716-446655440000"))
}

func TestNew_DefaultsToHashedKeyLogging(t *testing.T) {
	t.Parallel()

	m := New(nil)
	require.Equal(t, KeyLogHashed, m.opts.keyLogMode)
	require.Equal(t, KeyLogHashed, DefaultKeyLogMode)
}

func TestHashKeyForLog(t *testing.T) {
	t.Parallel()

	const key = "550e8400-e29b-41d4-a716-446655440000"

	got := hashKeyForLog(key)
	require.Len(t, got, keyLogHashLength)
	require.Equal(t, corehash.SHA256HexString(key)[:keyLogHashLength], got)
	require.Equal(t, got, hashKeyForLog(key), "hash must be stable so records correlate")
	require.NotEqual(t, got, hashKeyForLog(key+"x"))
}

// TestCheckIdempotency_InvalidKeyNotLoggedVerbatim pins the security contract:
// the invalid-format branch sees unvalidated caller bytes, and under the
// default mode they must not reach the log.
func TestCheckIdempotency_InvalidKeyNotLoggedVerbatim(t *testing.T) {
	t.Parallel()

	const rawKey = "not-a-uuid-super-secret"

	tests := []struct {
		name     string
		opts     []Option
		wantAttr string // exact key attribute the record must carry; "" means none
		wantRaw  bool   // whether the raw key may appear anywhere in the record
	}{
		{"default_mode_hashes", nil, "key_hash=" + hashKeyForLog(rawKey), false},
		{"hashed_mode_hashes", []Option{WithKeyLogMode(KeyLogHashed)}, "key_hash=" + hashKeyForLog(rawKey), false},
		{"off_mode_omits", []Option{WithKeyLogMode(KeyLogOff)}, "", false},
		{"full_mode_logs_raw", []Option{WithKeyLogMode(KeyLogFull)}, "key=" + rawKey, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			m := New(nil, append([]Option{WithLogger(debugLogger(&buf))}, tc.opts...)...)

			r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/users", nil)
			r.Header.Set(DefaultIdempotencyKeyHeader, rawKey)
			w := httptest.NewRecorder()

			_, _, err := m.checkIdempotency(w, r)
			require.ErrorIs(t, err, errIdempotencyKeyInvalidFormat)

			out := buf.String()
			require.Contains(t, out, "invalid idempotency key format", "the branch under test must have logged")

			// The security claim: the raw key reaches the log only under KeyLogFull.
			require.Equal(t, tc.wantRaw, strings.Contains(out, rawKey))

			if tc.wantAttr == "" {
				require.NotContains(t, out, "key=")
				require.NotContains(t, out, "key_hash=")
				return
			}
			require.Contains(t, out, tc.wantAttr)
		})
	}
}

// debugLogger returns a logger that emits debug records to w.
func debugLogger(w io.Writer) *slog.Logger {
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug}))
}
