// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package keys

import (
	"crypto/sha256"
	"encoding/binary"
	"strconv"
	"strings"
)

// Compose encodes an ordered tuple into one string: for every part, its length in bytes in decimal, ':' and its unchanged bytes.
// ("a:b", "c") becomes "3:a:b1:c" and ("a", "b:c") becomes "1:a3:b:c", so distinct tuples never share a key whatever bytes their
// parts hold. A part may be empty, and a part may itself be a composed key: it is framed like any other bytes. No parts give "".
//
// The layout is frozen: callers persist composed keys (data/idempotency prefixes them with its version marker).
func Compose(parts ...string) string {
	var scratch [20]byte // the decimal digits of a uint64

	size := 0
	for _, part := range parts {
		digits := strconv.AppendUint(scratch[:0], uint64(len(part)), 10)
		size += len(digits) + 1 + len(part)
	}

	var b strings.Builder
	b.Grow(size)
	for _, part := range parts {
		b.Write(strconv.AppendUint(scratch[:0], uint64(len(part)), 10))
		b.WriteByte(':')
		b.WriteString(part)
	}
	return b.String()
}

// Digest returns the SHA-256 of parts, each prefixed with its length as an 8-byte big-endian integer, so ("ab", "c") and
// ("a", "bc") differ and an empty part counts. Use it for fixed-size keys of variable input — request fingerprints, cache keys of
// long queries. The layout is frozen: callers persist digests (data/idempotency stores them as request fingerprints).
func Digest(parts ...[]byte) []byte {
	h := sha256.New()
	var n [8]byte
	for _, p := range parts {
		binary.BigEndian.PutUint64(n[:], uint64(len(p)))
		h.Write(n[:])
		h.Write(p)
	}
	return h.Sum(nil)
}
