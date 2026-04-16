// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import "time"

// WALConfig configures a write-ahead log backing an async dispatch engine.
// Shared by all subsystems that use WAL-backed dispatch (audit, reqlog, etc.).
type WALConfig struct {
	// Enabled turns on local WAL durability.
	Enabled bool `yaml:"enabled"`

	// Dir is the directory storing WAL segment files. It is created if missing.
	Dir string `yaml:"dir"`

	// MaxSegmentBytes is the maximum size of a single segment file.
	MaxSegmentBytes int64 `yaml:"maxSegmentBytes" default:"67108864"` // 64 MiB

	// MaxBytes is the soft cap on total bytes across all segments.
	MaxBytes int64 `yaml:"maxBytes" default:"1073741824"` // 1 GiB

	// FsyncInterval is the period between background fsync calls. A
	// shorter interval reduces the loss window after a crash, at the
	// cost of throughput.
	FsyncInterval time.Duration `yaml:"fsyncInterval" default:"5ms"`
}
