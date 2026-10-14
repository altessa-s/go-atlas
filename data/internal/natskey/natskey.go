// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natskey

import (
	"errors"
	"fmt"
	"strings"
)

// MaxLen is the longest physical key, in bytes, [Encode] produces.
//
// The NATS server limits a protocol control line to 4096 bytes by default
// (max_control_line). A KeyValue operation puts the key into that line behind
// the "$KV.<bucket>." subject prefix (or a JetStream API/domain prefix), next to
// a reply inbox, the command and the payload sizes. 3072 bytes leave a
// kilobyte for all of those.
const MaxLen = 3072

// escape introduces an escaped byte; it is never kept literally.
const escape = '='

// ErrKeyTooLong reports a key whose encoding exceeds [MaxLen] bytes.
var ErrKeyTooLong = errors.New("NATS KeyValue key too long")

// ErrMalformed reports a physical key [Decode] cannot invert.
var ErrMalformed = errors.New("malformed NATS KeyValue key")

// emptyKey is the encoding of the empty key; no other key encodes to it.
const emptyKey = "="

const hexDigits = "0123456789ABCDEF"

// Encode returns the physical NATS KeyValue key for key. See the package
// documentation for the scheme. A key that needs no escaping is returned
// unchanged without allocating.
func Encode(key string) (string, error) {
	if len(key) > MaxLen {
		return "", fmt.Errorf("%w: %d bytes, limit %d", ErrKeyTooLong, len(key), MaxLen)
	}
	if key == "" {
		return emptyKey, nil
	}

	n := len(key)
	for i := range len(key) {
		if !literal(key, i) {
			n += 2
		}
	}
	if n > MaxLen {
		return "", fmt.Errorf("%w: %d bytes encoded, limit %d", ErrKeyTooLong, n, MaxLen)
	}
	if n == len(key) {
		return key, nil
	}

	var b strings.Builder
	b.Grow(n)
	for i := range len(key) {
		c := key[i]
		if literal(key, i) {
			b.WriteByte(c)
			continue
		}
		b.WriteByte(escape)
		b.WriteByte(hexDigits[c>>4])
		b.WriteByte(hexDigits[c&0x0f])
	}
	return b.String(), nil
}

// Decode returns the logical key [Encode] mapped onto physical. It fails with
// [ErrMalformed] for a string Encode never produces.
func Decode(physical string) (string, error) {
	if physical == emptyKey {
		return "", nil
	}

	var b strings.Builder
	b.Grow(len(physical))
	for i := 0; i < len(physical); i++ {
		c := physical[i]
		if c != escape {
			b.WriteByte(c)
			continue
		}
		if i+2 >= len(physical) {
			return "", fmt.Errorf("%w: truncated escape in %q", ErrMalformed, physical)
		}
		hi, lo := unhex(physical[i+1]), unhex(physical[i+2])
		if hi < 0 || lo < 0 {
			return "", fmt.Errorf("%w: bad escape in %q", ErrMalformed, physical)
		}
		b.WriteByte(byte(hi<<4 | lo))
		i += 2
	}

	// Canonical form only: anything else would make two physical keys decode
	// to the same logical key.
	key := b.String()
	if enc, err := Encode(key); err != nil || enc != physical {
		return "", fmt.Errorf("%w: non-canonical %q", ErrMalformed, physical)
	}
	return key, nil
}

// literal reports whether key[i] is kept as is.
func literal(key string, i int) bool {
	c := key[i]
	switch {
	case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		return true
	case c == '-', c == '_', c == '/':
		return true
	case c == '.':
		// A dot separates subject tokens: it must not open or close the key
		// or follow another dot, either of which leaves an empty token.
		return i > 0 && i+1 < len(key) && key[i-1] != '.'
	default:
		return false
	}
}

// unhex returns the value of an upper-case hex digit, or -1.
func unhex(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10 //nolint:mnd // hex digit value
	default:
		return -1
	}
}
