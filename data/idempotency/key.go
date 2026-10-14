// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

import (
	"strconv"
	"strings"
)

// keyFormatPrefix marks a storage key composed by [ComposeKey] in the v2
// format.
const keyFormatPrefix = "idk:v2:"

// ComposeKey encodes an ordered tuple of components into one storage key in
// the persisted v2 format: "idk:v2:" followed, for every component, by its
// length in bytes in decimal, ':' and its unchanged bytes. The tuple
// ("a:b", "c") becomes "idk:v2:3:a:b1:c" and ("a", "b:c") becomes
// "idk:v2:1:a3:b:c", so distinct tuples never share a key whatever bytes
// their components hold. A component may be empty, and a component may
// itself be a composed key: it is framed like any other bytes.
//
// The bytes are an external ABI: they persist in storage backends. The v2
// layout is frozen; an incompatible layout needs a new version marker and a
// migration.
func ComposeKey(parts ...string) string {
	var scratch [20]byte // the decimal digits of a uint64

	size := len(keyFormatPrefix)
	for _, part := range parts {
		digits := strconv.AppendUint(scratch[:0], uint64(len(part)), 10)
		size += len(digits) + 1 + len(part)
	}

	var b strings.Builder
	b.Grow(size)
	b.WriteString(keyFormatPrefix)
	for _, part := range parts {
		b.Write(strconv.AppendUint(scratch[:0], uint64(len(part)), 10))
		b.WriteByte(':')
		b.WriteString(part)
	}
	return b.String()
}
