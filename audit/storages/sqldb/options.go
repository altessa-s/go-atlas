// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb

import (
	"log/slog"
	"time"

	"github.com/altessa-s/go-atlas-core/time/clock"
)

//go:generate go tool optgen generate

// DefaultTableName is the default table holding audit events; it matches the
// MongoDB and ClickHouse storages' default.
const DefaultTableName = "audit_events"

// DefaultMaxBatchRows is the default number of rows one INSERT of StoreBatch
// carries. Twelve parameters per row keep it well under PostgreSQL's limit
// of 65535 parameters per statement.
const DefaultMaxBatchRows = 500

// DefaultPurgeInterval is the interval between background retention passes.
const DefaultPurgeInterval = time.Minute

// DefaultPurgeBatchSize bounds the rows deleted by one retention statement.
const DefaultPurgeBatchSize = 1000

type options struct {
	tableName string `optval:"nonempty" optgen:"default=DefaultTableName"`
	// maxBatchRows bounds the rows of one INSERT; a larger batch is split
	// into several INSERTs of one transaction. It is capped so one INSERT
	// stays within PostgreSQL's 65535 bind parameters.
	maxBatchRows int `optval:"positive" optgen:"default=DefaultMaxBatchRows"`
	// WithTTL sets the event retention period. Zero (the default) keeps events
	// forever. Nonpositive values leave the configured TTL unchanged, as in mongo.
	ttl time.Duration `opt:"TTL" optval:"positive"`
	// WithPurgeInterval sets the interval between retention passes. The first
	// pass runs after this interval. Zero disables the background loop.
	purgeInterval time.Duration `optcheck:"min=0" optgen:"default=DefaultPurgeInterval,onfail=skip"`
	// WithPurgeBatchSize bounds the rows deleted by each statement. A retention
	// pass continues until fewer rows are deleted. Nonpositive values are ignored.
	purgeBatchSize int `optval:"positive" optgen:"default=DefaultPurgeBatchSize"`
	// WithClock sets the application clock used for retention cutoffs. The
	// background interval uses a real timer independently of this clock.
	clock clock.Clock `optgen:"default=clock.Real(),notnil"`
	// WithLogger sets the logger for background retention failures and panics.
	// Logging is discarded by default; nil leaves the configured logger unchanged.
	logger *slog.Logger `optgen:"default=slog.New(slog.DiscardHandler),notnil"`
}
