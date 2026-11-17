// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb

//go:generate go tool optgen generate

import (
	"log/slog"
	"time"

	"github.com/altessa-s/go-atlas-core/encoding/serializer"
)

// DefaultTableName is the default table name for retry items.
const DefaultTableName = "retry_items"

// DefaultPurgeInterval is the interval between background purge passes.
const DefaultPurgeInterval = time.Minute

// DefaultPurgeBatchSize bounds the rows deleted by one purge statement.
const DefaultPurgeBatchSize = 1000

type options struct {
	// tableName is the items table, optionally qualified as schema.name.
	tableName string `optval:"nonempty" optgen:"default=DefaultTableName"`
	// codec encodes the payload into the payload column and decodes it back.
	// The default is JSON.
	codec serializer.Serializer `optgen:"notnil,default=serializer.NewJSON()"`
	// resolvedRetention is how long [Store.PurgeExpired] keeps a resolved
	// item, measured against the database clock. Zero, the default, keeps
	// resolved items until they are deleted.
	resolvedRetention time.Duration `optval:"positive"`
	// WithPurgeInterval sets the interval between purge passes. Zero disables
	// the background loop; manual PurgeExpired calls remain available.
	purgeInterval time.Duration `optcheck:"min=0" optgen:"default=DefaultPurgeInterval,onfail=skip"`
	// WithPurgeBatchSize bounds the rows deleted by each statement.
	// Nonpositive values leave the configured size unchanged.
	purgeBatchSize int `optval:"positive" optgen:"default=DefaultPurgeBatchSize"`
	// logger receives the background purge's failures, recovered panics and
	// counts. The default discards them.
	logger *slog.Logger `optgen:"default=slog.New(slog.DiscardHandler),notnil"`
}
