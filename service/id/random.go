// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package id

import (
	"crypto/rand"
	"encoding/hex"
)

// NewHex returns n bytes of crypto/rand entropy as 2n lowercase hex digits, for server-assigned resource IDs and one-off tokens
// such as lease owners: NewHex(16) carries 128 bits. It never fails (crypto/rand.Read does not return an error since Go 1.24) and
// returns "" for n == 0; it panics for a negative n.
func NewHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
