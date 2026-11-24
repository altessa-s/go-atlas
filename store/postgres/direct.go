// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// ErrDirectConnectionRequired indicates that a session cannot be verified as direct.
var ErrDirectConnectionRequired = errors.New("direct PostgreSQL connection required")

// CheckDirect verifies the session identity and, when supplied, the expected database.
// PostgreSQL sends its backend PID at startup. PgBouncer supplies a virtual cancellation key.
// Verify the session before acquiring any session-level lock.
func CheckDirect(ctx context.Context, c *pgx.Conn, database string) error {
	var pid uint32
	var current string
	if err := c.QueryRow(ctx, "SELECT pg_backend_pid(),current_database()").Scan(&pid, &current); err != nil {
		return err
	}
	if pid != c.PgConn().PID() || (database != "" && current != database) {
		return ErrDirectConnectionRequired
	}
	return nil
}
