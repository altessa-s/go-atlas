// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package spool

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

import "io"

// DefaultMemThreshold is the in-memory ceiling [New] uses when no
// [WithMemThreshold] override is supplied. Content up to this size is held in
// a byte slice; larger content spills to a temp file.
const DefaultMemThreshold int64 = 4 << 20 // 4 MiB

// options holds the tunables for [New].
type options struct {
	// memThreshold is the maximum number of bytes to hold in memory before
	// spilling to a temp file.
	memThreshold int64 `optgen:"default=DefaultMemThreshold"`
	// maxBytes caps the total materialized size. Zero means unlimited.
	maxBytes int64
	// tee mirrors every byte read from the source into this writer during
	// materialization so callers can compute a digest in one pass.
	tee io.Writer
}
