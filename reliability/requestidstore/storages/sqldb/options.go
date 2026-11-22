// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb

import (
	"log/slog"
	"time"
)

//go:generate go tool optgen generate

// DefaultTableName holds AIP-155 request ID entries.
const DefaultTableName = "request_id_entries"

// DefaultPurgeInterval is the interval between background collection passes.
const DefaultPurgeInterval = time.Minute

// DefaultPurgeBatchSize bounds the rows removed by one statement.
const DefaultPurgeBatchSize = 1000

// DefaultCleanupGrace retains expired entries for an extra hour before physical cleanup.
const DefaultCleanupGrace = time.Hour

type options struct {
	// WithCleanupGrace sets the retention margin for clock skew. Values below one millisecond are ignored.
	cleanupGrace time.Duration `optgen:"default=DefaultCleanupGrace,onfail=skip" optcheck:"min=time.Millisecond"`
	tableName    string        `optval:"nonempty" optgen:"default=DefaultTableName"`
	// WithPurgeInterval sets the interval between background passes. Zero disables them.
	purgeInterval  time.Duration `optcheck:"min=0" optgen:"default=DefaultPurgeInterval,onfail=skip"`
	purgeBatchSize int           `optval:"positive" optgen:"default=DefaultPurgeBatchSize"`
	// WithLogger sets the logger for background purge errors.
	logger *slog.Logger `optgen:"default=slog.New(slog.DiscardHandler),notnil"`
}
