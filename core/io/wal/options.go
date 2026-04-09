// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package wal

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate

import (
	"log/slog"
	"time"
)

// Default tunables applied by [Open] when the corresponding option is not set.
const (
	// DefaultMaxSegmentBytes is the default roll-over size for a segment
	// file. See [WithMaxSegmentBytes].
	DefaultMaxSegmentBytes int64 = 64 << 20
	// DefaultMaxBytes is the default soft cap on total bytes held across
	// all segments. A value of zero (the default) disables the cap entirely.
	// See [WithMaxBytes].
	DefaultMaxBytes int64 = 0
	// DefaultFsyncInterval is the default period between background fsync
	// calls on the active segment. See [WithFsyncInterval].
	DefaultFsyncInterval = 5 * time.Millisecond
)

// options holds the tunables of a [WAL]. Callers configure a WAL by
// passing [Option] values to [Open]; the required on-disk directory is
// supplied separately as a positional argument and therefore intentionally
// lives on the [WAL] struct rather than in this options bag.
type options struct {
	// maxSegmentBytes is the maximum size of a single segment file before
	// rolling over to a new one.
	maxSegmentBytes int64 `optgen:"default=DefaultMaxSegmentBytes"`
	// maxBytes is the soft cap on total bytes held across all segments.
	// When exceeded, Append returns ErrFull. Zero disables the cap.
	maxBytes int64
	// fsyncInterval is the period between background fsync calls on the
	// active segment. Shorter intervals reduce the loss window after a
	// crash at the cost of throughput.
	fsyncInterval time.Duration `optgen:"default=DefaultFsyncInterval"`
	// logger is the structured logger used to surface background fsync
	// errors that would otherwise be silently dropped. When nil, a
	// discard handler is installed by [Open] so call sites can emit
	// logs unconditionally.
	logger *slog.Logger
}
