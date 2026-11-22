// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package document

import (
	"errors"
	"fmt"
	"strings"
)

// CSFLE algorithm identifiers accepted by libmongocrypt.
const (
	Deterministic = "AEAD_AES_256_CBC_HMAC_SHA_512-Deterministic"
	Random        = "AEAD_AES_256_CBC_HMAC_SHA_512-Random"
)

// ErrUnknownAlgorithm reports an unrecognized encryption algorithm.
var ErrUnknownAlgorithm = errors.New("unknown encryption algorithm")

// Algorithm resolves a CSFLE algorithm or its case-insensitive short alias.
func Algorithm(name string) (string, error) {
	switch {
	case name == Deterministic, strings.EqualFold(name, "deterministic"):
		return Deterministic, nil
	case name == Random, strings.EqualFold(name, "random"):
		return Random, nil
	default:
		return "", fmt.Errorf("%w: unsupported encryption algorithm %q", ErrUnknownAlgorithm, name)
	}
}

// EncryptionField defines how a specific struct field should be encrypted.
type EncryptionField struct {
	// FieldName is the name of the struct field to encrypt (case-sensitive)
	FieldName string
	// Algorithm is the encryption algorithm: "deterministic" or "random"
	Algorithm string
	// KeyAltName is the alternative name of the data key to use for encryption
	KeyAltName string
}

// EncryptionModel defines encryption configuration for a specific struct type.
type EncryptionModel struct {
	Type any
	// Fields defines which fields should be encrypted and how
	Fields []EncryptionField
}
