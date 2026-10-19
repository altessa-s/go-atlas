// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sealbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"

	coremaps "github.com/altessa-s/go-atlas/core/collections/maps"
)

// KeyLen is the length of a master key in bytes (AES-256).
const KeyLen = 32

// KeyIDLen is the length of a key identifier: the first hex digits of the SHA-256 of the raw key.
const KeyIDLen = 16

// Sealed is an encrypted value: the identifier of the key that sealed it, the GCM nonce and the ciphertext (with the GCM tag appended).
type Sealed struct {
	// KeyID identifies the key the value was sealed with ([KeyID]).
	KeyID string
	// Nonce is the random GCM nonce (12 bytes).
	Nonce []byte
	// Ciphertext is the GCM output: the encrypted plaintext followed by the 16-byte tag.
	Ciphertext []byte
}

// Set reports whether the sealed value carries ciphertext.
func (s Sealed) Set() bool { return len(s.Ciphertext) > 0 }

// Box seals and opens values with AES-256-GCM. The current key seals; previous keys still open values sealed before a rotation. A nil
// Box has no key: sealing and opening fail with [ErrNoKey]. A Box is immutable and safe for concurrent use.
type Box struct {
	current string
	aeads   *coremaps.ImmutableMap[string, cipher.AEAD]
}

// New returns a box sealing with current and opening with current and previous. Keys are standard base64 encodings of 32-byte values;
// surrounding whitespace is ignored. An empty (or blank) current key returns a nil box and no error: no key is configured, and the box's
// methods report [ErrNoKey]. A malformed key fails with an error naming its position (0 = current), never its value.
func New(current string, previous ...string) (*Box, error) {
	if strings.TrimSpace(current) == "" {
		return nil, nil //nolint:nilnil // no key configured is not an error: the nil box reports ErrNoKey
	}
	aeads := make(map[string]cipher.AEAD, 1+len(previous))
	var currentID string
	for i, k := range append([]string{current}, previous...) {
		id, aead, err := load(k)
		if err != nil {
			return nil, fmt.Errorf("sealbox: key %d: %w", i, err)
		}
		if i == 0 {
			currentID = id
		}
		aeads[id] = aead
	}
	return &Box{current: currentID, aeads: coremaps.NewImmutableMap(aeads)}, nil
}

// KeyID returns the identifier of a raw key: the first [KeyIDLen] hex digits of its SHA-256. It is what [Sealed.KeyID] holds.
func KeyID(key []byte) string {
	sum := sha256.Sum256(key)
	return hex.EncodeToString(sum[:])[:KeyIDLen]
}

func load(encoded string) (string, cipher.AEAD, error) {
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return "", nil, fmt.Errorf("not base64: %w", err)
	}
	if len(key) != KeyLen {
		return "", nil, fmt.Errorf("%d bytes, want %d", len(key), KeyLen)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", nil, err
	}
	return KeyID(key), aead, nil
}

// Seal encrypts plaintext with the current key under a fresh random nonce, binding it to aad: the value opens only with the same
// associated data.
func (b *Box) Seal(plaintext, aad []byte) (Sealed, error) {
	if b == nil {
		return Sealed{}, ErrNoKey
	}
	aead, _ := b.aeads.Get(b.current)
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return Sealed{}, fmt.Errorf("sealbox: nonce: %w", err)
	}
	return Sealed{KeyID: b.current, Nonce: nonce, Ciphertext: aead.Seal(nil, nonce, plaintext, aad)}, nil
}

// Open decrypts a sealed value with its key and associated data. A key that is not configured fails with [ErrUnknownKey]; a value
// that does not authenticate (tampered, wrong associated data, malformed nonce) fails with [ErrOpen].
func (b *Box) Open(s Sealed, aad []byte) ([]byte, error) {
	if b == nil {
		return nil, ErrNoKey
	}
	aead, ok := b.aeads.Get(s.KeyID)
	if !ok {
		return nil, ErrUnknownKey
	}
	if len(s.Nonce) != aead.NonceSize() {
		return nil, ErrOpen
	}
	out, err := aead.Open(nil, s.Nonce, s.Ciphertext, aad)
	if err != nil {
		return nil, ErrOpen
	}
	return out, nil
}

// Current reports whether a value was sealed with the current key. Values sealed with a previous key should be re-sealed on their next
// save so the previous key can eventually be dropped.
func (b *Box) Current(s Sealed) bool {
	return b != nil && s.KeyID == b.current
}

// Opens reports whether the box holds the key s was sealed with (the current or a previous one). It does not authenticate the value.
func (b *Box) Opens(s Sealed) bool {
	if b == nil {
		return false
	}
	return b.aeads.Contains(s.KeyID)
}
