// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldialect

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// MaxKeyBatch caps a batch of keys bound one placeholder each through
// [InList]: MySQL allows 65,535 placeholders per prepared statement, and the
// statement binds one more argument.
const MaxKeyBatch = 65_534

// Querier runs a query; *sql.DB, *sql.Conn and *sql.Tx implement it.
type Querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// QueryKeys runs query and returns its single binary column, one []byte per
// row, as bind arguments for a statement built with [InList]. The result has
// room for limit keys and one more argument, so a caller that appends one
// does not reallocate.
func QueryKeys(ctx context.Context, q Querier, limit int, query string, args ...any) ([]any, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	keys := make([]any, 0, max(limit, 0)+1)
	for rows.Next() {
		var key []byte
		if err := rows.Scan(&key); err != nil {
			_ = rows.Close()
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, errors.Join(rows.Err(), rows.Close())
}

// InList replaces the single %s verb in stmt with n comma-separated ?
// placeholders, for an IN list of n keys. n must be positive.
func InList(stmt string, n int) string {
	return fmt.Sprintf(stmt, strings.Repeat(", ?", n)[2:])
}
