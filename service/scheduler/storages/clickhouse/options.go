// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package clickhouse

import (
	"time"

	"github.com/altessa-s/go-atlas/service/scheduler"
)

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate --type=options

const (
	// DefaultTableName is the table holding [scheduler.TaskHistory] rows when no
	// override is provided via [WithTableName].
	DefaultTableName = "scheduler_history"

	// DefaultEngine is the table engine [SchemaDDL] renders when no override is
	// provided via [WithEngine]. History rows are never updated, so the plain
	// MergeTree is enough.
	DefaultEngine = "MergeTree"

	// DefaultTTL is the retention [SchemaDDL] renders as the table TTL when no
	// override is provided via [WithTTL]: the scheduler's default history
	// retention.
	DefaultTTL = scheduler.DefaultHistoryRetention
)

type options struct {
	tableName string `optval:"nonempty" optgen:"default=DefaultTableName"`
	engine    string `optval:"nonempty" optgen:"default=DefaultEngine"`
	// ttl is the table TTL after the end of a run. A non-positive value
	// renders no TTL clause: history is then kept indefinitely.
	ttl time.Duration `opt:"TTL" optval:"nonpositive" optgen:"default=DefaultTTL"`

	// cluster adds an ON CLUSTER clause to the DDL and to DeleteHistory. Empty
	// means a single-node table. Pair it with a Replicated* engine: ON CLUSTER
	// only distributes the statement, not the data.
	cluster string
}
