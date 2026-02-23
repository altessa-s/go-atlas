// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package base64

import (
	"encoding/base64"

	"github.com/altessa-s/go-atlas/security/secrets/codec"
)

// Ensure KeyDecoder implements codec.KeyDecoder interface
var _ codec.KeyDecoder = (*KeyDecoder)(nil)

// KeyDecoder implements codec.KeyDecoder using base64 encoding.
// It uses RawURLEncoding to ensure safety in URL paths and
// prevent problems with special characters in keys.
type KeyDecoder struct{}

// NewKeyDecoder creates a new KeyDecoder instance.
// This function provides a convenient way to create KeyDecoder instances
// with proper type constraints.
//
// Returns a new KeyDecoder instance configured for base64 URL-safe encoding.
func NewKeyDecoder() *KeyDecoder {
	return &KeyDecoder{}
}

// Decode decodes a key from base64 format to its original representation.
// RawURLEncoding is used to handle URL-safe base64 strings without padding.
//
// Parameters:
//   - key: base64 encoded string that needs to be decoded
//
// Returns:
//   - the decoded original key
//   - an error if the input string is not a valid base64 string
func (KeyDecoder) Decode(key string) (string, error) {
	dKey, err := base64.RawURLEncoding.DecodeString(key)
	if err != nil {
		return "", err
	}
	return string(dKey), nil
}

// Encode encodes an original key to base64 format.
// RawURLEncoding is used to create a URL-safe base64 string without padding.
//
// Parameters:
//   - key: original string that needs to be encoded
//
// Returns:
//   - the base64 encoded key
//   - always nil, as the encoding operation cannot fail
func (KeyDecoder) Encode(key string) (string, error) {
	encodedKey := base64.RawURLEncoding.EncodeToString([]byte(key))
	return encodedKey, nil
}
