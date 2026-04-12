// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package secrets_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/security/secrets"
)

func TestValidateKeyLength(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		expected bool
	}{
		{"empty string", "", false},
		{"one character", "a", false},
		{"two characters - minimum valid", "ab", true},
		{"normal length key", "my-secret-key", true},
		{"255 characters - maximum valid", strings.Repeat("a", 255), true},
		{"256 characters - exceeds maximum", strings.Repeat("a", 256), false},
		{"whitespace only - single space", " ", false},
		{"whitespace only - multiple spaces", "   ", false},
		{"whitespace only - tabs", "\t\t", false},
		{"whitespace only - mixed", " \t \n ", false},
		{"key with spaces in middle", "my key", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := secrets.ValidateKeyLength(tt.key)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestCreateLockKey(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		key      string
		expected string
	}{
		{"basic", "vault", "my-key", "vault:my-key"},
		{"empty provider", "", "my-key", ":my-key"},
		{"empty key", "vault", "", "vault:"},
		{"both empty", "", "", ":"},
		{"special chars", "aws-secrets", "prod.key", "aws-secrets:prod.key"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := secrets.CreateLockKey(tt.provider, tt.key)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestValidateSecretKey(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{"empty string", "", true},
		{"one character", "a", true},
		{"two characters - minimum valid", "ab", false},
		{"255 characters - maximum valid", strings.Repeat("a", 255), false},
		{"256 characters - exceeds maximum", strings.Repeat("a", 256), true},
		{"valid alphanumeric", "mykey123", false},
		{"valid with underscore", "my_key", false},
		{"valid with dot", "my.key", false},
		{"valid with dash", "my-key", false},
		{"valid complex key", "my.key_name-1", false},
		{"valid all allowed chars", "aZ09_.-", false},
		{"invalid with space", "my key", true},
		{"invalid with exclamation", "my!key", true},
		{"invalid with at symbol", "my@key", true},
		{"invalid with hash", "my#key", true},
		{"invalid with dollar sign", "my$key", true},
		{"invalid with percent", "my%key", true},
		{"invalid with slash", "my/key", true},
		{"invalid with backslash", "my\\key", true},
		{"whitespace only", "   ", true},
		{"leading spaces", " mykey", true},
		{"trailing spaces", "mykey ", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := secrets.ValidateSecretKey(tt.key)
			if tt.wantErr {
				require.ErrorIs(t, err, secrets.ErrInvalidKey)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
