// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package migrations

//go:generate go tool optgen generate --keep-empty-strings

import (
	"log/slog"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

// Defaults of [Run], [Down] and [Status].
const (
	// DefaultTableName is the table recording the applied versions: goose's own, so a schema migrated with the goose tool before
	// keeps its history.
	DefaultTableName = goose.DefaultTablename
	// DefaultLockID is the key of the PostgreSQL session-level advisory lock that serializes [Run] and [Down] across processes.
	DefaultLockID = lock.DefaultLockID
	// DefaultLockTimeout bounds waiting for that lock, independently of the migration work, which the caller's context bounds.
	DefaultLockTimeout = 30 * time.Second
)

// options configures [Run], [Down] and [Status].
type options struct {
	// WithLogger selects the logger for goose migration progress. The default discards messages.
	logger *slog.Logger
	// WithTableName names the table recording the applied versions.
	tableName string `optval:"notrim" optgen:"default=DefaultTableName"`
	// WithLockID sets the key of the advisory lock taken for the run; processes migrating one database must share it.
	lockID int64 `optgen:"default=DefaultLockID"`
	// WithLockTimeout bounds connection checkout, identity verification and advisory lock acquisition.
	lockTimeout time.Duration `optval:"positive" optgen:"default=DefaultLockTimeout"`
}
