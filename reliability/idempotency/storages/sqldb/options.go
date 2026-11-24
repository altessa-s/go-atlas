// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb

import (
	"context"
	"database/sql"
	"log/slog"
	"time"
)

//go:generate go tool optgen generate

// DefaultTableName is the table in which New stores idempotency keys.
const DefaultTableName = "idempotency_keys"

// DefaultTTL is the result lifetime and default lock lifetime.
const DefaultTTL = 24 * time.Hour

// DefaultPurgeInterval is the interval between background purge passes.
const DefaultPurgeInterval = time.Minute

// DefaultPurgeBatchSize bounds the rows deleted by one purge statement.
const DefaultPurgeBatchSize = 1000

type options struct {
	// WithTxResolver finds the application transaction for a call.
	txResolver func(context.Context) (*sql.Tx, bool) `optgen:"notnil"`
	tableName  string                                `optval:"nonempty" optgen:"default=DefaultTableName"`
	ttl        time.Duration                         `optgen:"default=DefaultTTL"`
	// WithPurgeInterval sets the interval between purge passes. Zero disables
	// the background loop; manual PurgeExpired calls remain available.
	purgeInterval time.Duration `optcheck:"min=0" optgen:"default=DefaultPurgeInterval,onfail=skip"`
	// WithPurgeBatchSize bounds the rows deleted by each statement. On MySQL it
	// must be at most 65534 (the placeholder limit of the key list), or [New]
	// fails. Nonpositive values leave the configured size unchanged.
	purgeBatchSize int `optval:"positive" optgen:"default=DefaultPurgeBatchSize"`
	// WithLogger sets the logger used for background cleanup errors and panics.
	logger *slog.Logger `optgen:"default=slog.New(slog.DiscardHandler),notnil"`
}
