// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package base64_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/security/secrets/codec/keys/base64"
)

func TestKeyDecoder_EncodeDecode(t *testing.T) {
	tests := []struct {
		name     string
		original string
	}{
		{"empty string", ""},
		{"simple string", "hello"},
		{"special chars", "key.with-special_chars"},
		{"URL-unsafe chars", "a/b+c=d"},
	}

	decoder := base64.NewKeyDecoder()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := decoder.Encode(tt.original)
			require.NoError(t, err)
			decoded, err := decoder.Decode(encoded)
			require.NoError(t, err)
			require.Equal(t, tt.original, decoded)
		})
	}
}

func TestKeyDecoder_Decode_Invalid(t *testing.T) {
	decoder := base64.NewKeyDecoder()
	_, err := decoder.Decode("!!!invalid@base64#string$$$")
	require.Error(t, err)
}

func TestKeyDecoder_Encode_NeverErrors(t *testing.T) {
	decoder := base64.NewKeyDecoder()
	inputs := []string{"", "simple", "with spaces", "unicode: 日本語", "special!@#$%^&*()"}
	for _, input := range inputs {
		_, err := decoder.Encode(input)
		require.NoError(t, err)
	}
}
