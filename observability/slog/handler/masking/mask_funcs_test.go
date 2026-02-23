// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package masking_test

import (
	"strings"
	"testing"

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
		if got != tt.want {
			t.Errorf("PartialMask(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSmartMask(t *testing.T) {
	mask := masking.SmartMask()
	got := mask("secret123")
	if !strings.Contains(got, "se") || !strings.Contains(got, "23") {
		t.Errorf("SmartMask(secret123) = %q, expected first 2 and last 2 visible", got)
	}
}

func TestFullMask(t *testing.T) {
	mask := masking.FullMask()
	got := mask("anything")
	if got != "********" {
		t.Errorf("FullMask() = %q, want ********", got)
	}
}

func TestFixedMask(t *testing.T) {
	mask := masking.FixedMask("[REDACTED]")
	got := mask("secret")
	if got != "[REDACTED]" {
		t.Errorf("FixedMask() = %q, want [REDACTED]", got)
	}
}

func TestEmailMask(t *testing.T) {
	mask := masking.EmailMask()
	got := mask("user@example.com")
	if !strings.Contains(got, "@example.com") {
		t.Errorf("EmailMask(user@example.com) = %q, expected domain preserved", got)
	}
	if strings.HasPrefix(got, "user@") {
		t.Errorf("EmailMask should mask local part, got %q", got)
	}
}

func TestPhoneMask(t *testing.T) {
	mask := masking.PhoneMask()
	got := mask("+1234567890")
	if got == "+1234567890" {
		t.Errorf("PhoneMask should mask the number, got %q", got)
	}
}

func TestCreditCardMask(t *testing.T) {
	mask := masking.CreditCardMask()
	got := mask("4111111111111111")
	if got == "4111111111111111" {
		t.Errorf("CreditCardMask should mask the number, got %q", got)
	}
}

func TestHashMask(t *testing.T) {
	mask := masking.HashMask("hash:")
	got := mask("secret")
	if !strings.HasPrefix(got, "hash:") {
		t.Errorf("HashMask() = %q, want prefix hash:", got)
	}
}

func TestPatternMask(t *testing.T) {
	mask := masking.PatternMask(`\d+`, masking.FixedMask("***"))
	got := mask("order-12345-abc")
	if strings.Contains(got, "12345") {
		t.Errorf("PatternMask should mask digits, got %q", got)
	}
}

func TestCachedPartialMask(t *testing.T) {
	mask := masking.CachedPartialMask(2, 2, "*")
	got := mask("abcdefgh")
	if !strings.HasPrefix(got, "ab") || !strings.HasSuffix(got, "gh") {
		t.Errorf("CachedPartialMask(abcdefgh) = %q", got)
	}
}

func TestPrecomputedMasks(t *testing.T) {
	pm := masking.NewPrecomputedMasks("*")
	got := pm.GetMask(3)
	if got != "***" {
		t.Errorf("GetMask(3) = %q, want ***", got)
	}
	got = pm.GetMask(10)
	if len(got) != 10 {
		t.Errorf("GetMask(10) len = %d, want 10", len(got))
	}
}
