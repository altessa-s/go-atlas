// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tlsvault

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"sync"
)

// DefaultRSAKeyBits is the default RSA key size in bits.
const DefaultRSAKeyBits = 2048

// RSAGenerator generates RSA private keys for certificates.
// Keys are generated once and cached for reuse.
type RSAGenerator struct {
	key  crypto.PrivateKey
	err  error
	o    sync.Once
	bits int
}

// NewRSAGenerator creates a new RSA key generator with default key size.
//
// Example:
//
//	gen := NewRSAGenerator()
//	key, err := gen.Generate()
func NewRSAGenerator() *RSAGenerator {
	return &RSAGenerator{
		bits: DefaultRSAKeyBits,
	}
}

// Generate creates a new RSA private key.
// The key is generated only once and cached for subsequent calls.
func (s *RSAGenerator) Generate() (crypto.PrivateKey, error) {
	s.o.Do(func() {
		s.key, s.err = rsa.GenerateKey(rand.Reader, s.bits)
	})
	return s.key, s.err
}
