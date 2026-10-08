// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

import (
	"crypto/sha256"
	"encoding/binary"
)

// Fingerprint returns the SHA-256 of parts, each prefixed with its length,
// so that ("ab", "c") and ("a", "bc") differ. Pass the operation and a
// canonical encoding of the request — for protobuf, a deterministic marshal —
// as [AttemptLockOpts.Fingerprint].
func Fingerprint(parts ...[]byte) []byte {
	h := sha256.New()
	var n [8]byte
	for _, p := range parts {
		binary.BigEndian.PutUint64(n[:], uint64(len(p)))
		h.Write(n[:])
		h.Write(p)
	}
	return h.Sum(nil)
}
