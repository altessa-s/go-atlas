// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package masking_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/slog/handler/masking"
)

func TestPartialMask(t *testing.T) {
	mask := masking.PartialMask(2, 2, "*")
	tests := []struct {
		input string
		want  string
	}{
		{"abcdef", "ab**ef"},
		{"ab", "**"},
		{"a", "*"},
		{"", ""},
	}
	for _, tt := range tests {
		got := mask(tt.input)
		require.Equal(t, tt.want, got, "PartialMask(%q)", tt.input)
	}
}

func TestSmartMask(t *testing.T) {
	mask := masking.SmartMask()
	got := mask("secret123")
	require.Contains(t, got, "se")
	require.Contains(t, got, "23")
}

func TestFullMask(t *testing.T) {
	mask := masking.FullMask()
	got := mask("anything")
	require.Equal(t, "********", got)
}

func TestFixedMask(t *testing.T) {
	mask := masking.FixedMask("[REDACTED]")
	got := mask("secret")
	require.Equal(t, "[REDACTED]", got)
}

func TestEmailMask(t *testing.T) {
	mask := masking.EmailMask()
	got := mask("user@example.com")
	require.Contains(t, got, "@example.com")
	require.False(t, strings.HasPrefix(got, "user@"), "EmailMask should mask local part, got %q", got)
}

func TestPhoneMask(t *testing.T) {
	mask := masking.PhoneMask()
	got := mask("+1234567890")
	require.NotEqual(t, "+1234567890", got, "PhoneMask should mask the number")
}

func TestCreditCardMask(t *testing.T) {
	mask := masking.CreditCardMask()
	got := mask("4111111111111111")
	require.NotEqual(t, "4111111111111111", got, "CreditCardMask should mask the number")
}

func TestHashMask(t *testing.T) {
	mask := masking.HashMask("hash:")
	got := mask("secret")
	require.True(t, strings.HasPrefix(got, "hash:"), "HashMask() = %q, want prefix hash:", got)
}

func TestPatternMask(t *testing.T) {
	mask := masking.PatternMask(`\d+`, masking.FixedMask("***"))
	got := mask("order-12345-abc")
	require.NotContains(t, got, "12345", "PatternMask should mask digits")
}

func TestCachedPartialMask(t *testing.T) {
	mask := masking.CachedPartialMask(2, 2, "*")
	got := mask("abcdefgh")
	require.True(t, strings.HasPrefix(got, "ab"), "CachedPartialMask(abcdefgh) = %q", got)
	require.True(t, strings.HasSuffix(got, "gh"), "CachedPartialMask(abcdefgh) = %q", got)
}

func TestPrecomputedMasks(t *testing.T) {
	pm := masking.NewPrecomputedMasks("*")
	got := pm.GetMask(3)
	require.Equal(t, "***", got)
	got = pm.GetMask(10)
	require.Len(t, got, 10)
}
