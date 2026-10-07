// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb

import (
	"context"

	"github.com/altessa-s/go-atlas/internal/sqldialect"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// EnsureSchema creates the locks table if it does not exist. It is idempotent
// and safe to run concurrently from several instances; call it once at
// startup (or apply the same DDL through your migration tool). The table
// needs no index besides its primary key.
//
// On MySQL/MariaDB the key and owner are binary types, so keys compare exactly
// — byte-wise, no trailing space padding, no case folding — whatever the
// database defaults are. On PostgreSQL the DDL runs under an advisory lock on
// the table name, so instances creating the same absent table at once take
// turns instead of colliding in the catalog.
func (l *Locker) EnsureSchema(ctx context.Context) error {
	if l.s.dialect.name == DialectPostgres {
		return coreerrs.WrapOperation(sqldialect.ExecPostgresDDL(ctx, l.s.db, []string{l.s.tableName}, []string{l.postgresSchema()}),
			"create dlock schema")
	}
	if _, err := l.s.db.ExecContext(ctx, l.mysqlSchema()); err != nil {
		return coreerrs.WrapOperation(err, "create dlock schema")
	}
	return nil
}

// postgresSchema is the PostgreSQL DDL. Times are server-clock Unix
// microseconds.
func (l *Locker) postgresSchema() string {
	return `CREATE TABLE IF NOT EXISTS ` + l.s.table + ` (
  lock_key    VARCHAR(255) COLLATE "C" PRIMARY KEY,
  owner       TEXT   NOT NULL DEFAULT '',
  fencing     BIGINT NOT NULL DEFAULT 0,
  acquired_at BIGINT NOT NULL DEFAULT 0,
  renewed_at  BIGINT NOT NULL DEFAULT 0,
  expires_at  BIGINT NOT NULL DEFAULT 0,
  ttl_us      BIGINT NOT NULL DEFAULT 0
)`
}

// mysqlSchema declares the key and owner as binary types; the key width is in
// bytes, four per character of [MaxKeyLength].
func (l *Locker) mysqlSchema() string {
	return `CREATE TABLE IF NOT EXISTS ` + l.s.table + ` (
  lock_key    VARBINARY(1020) NOT NULL PRIMARY KEY,
  owner       VARBINARY(64)   NOT NULL DEFAULT '',
  fencing     BIGINT          NOT NULL DEFAULT 0,
  acquired_at BIGINT          NOT NULL DEFAULT 0,
  renewed_at  BIGINT          NOT NULL DEFAULT 0,
  expires_at  BIGINT          NOT NULL DEFAULT 0,
  ttl_us      BIGINT          NOT NULL DEFAULT 0
) ENGINE=InnoDB`
}
