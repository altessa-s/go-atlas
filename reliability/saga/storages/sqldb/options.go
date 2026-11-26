// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb

//go:generate go tool optgen generate

import (
	"context"
	"database/sql"
)

// DefaultTableName is the default table name for saga instances; it matches the
// MongoDB store's default collection name.
const DefaultTableName = "saga_instances"

type options struct {
	// WithTxResolver finds the application transaction for a call.
	txResolver func(context.Context) (*sql.Tx, bool, error) `optgen:"notnil"`
	tableName  string                                       `optval:"nonempty" optgen:"default=DefaultTableName"`
}
