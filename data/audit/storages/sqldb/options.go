// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

// DefaultTableName is the default table holding audit events; it matches the
// MongoDB and ClickHouse storages' default.
const DefaultTableName = "audit_events"

// DefaultMaxBatchRows is the default number of rows one INSERT of StoreBatch
// carries. Twelve parameters per row keep it well under PostgreSQL's limit
// of 65535 parameters per statement.
const DefaultMaxBatchRows = 500

type options struct {
	tableName string `optval:"nonempty" optgen:"default=DefaultTableName"`
	// maxBatchRows bounds the rows of one INSERT; a larger batch is split
	// into several INSERTs of one transaction. It is capped so one INSERT
	// stays within PostgreSQL's 65535 bind parameters.
	maxBatchRows int `optval:"positive" optgen:"default=DefaultMaxBatchRows"`
}
