// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb

import (
	"context"

	"github.com/altessa-s/go-atlas/internal/sqldialect"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// EnsureSchema creates the instances table and its recovery indexes if they do
// not exist. It is idempotent and safe to run concurrently from several
// instances; call it once at startup (or apply the same DDL through your
// migration tool).
//
// On MySQL/MariaDB every string column is a binary type (VARBINARY,
// MEDIUMBLOB, LONGBLOB), so IDs compare exactly — byte-wise, no trailing space
// padding, no case folding — whatever the database defaults are. On PostgreSQL
// the DDL runs in one transaction under an advisory lock on the table name, so
// instances creating the same absent table at once take turns instead of
// colliding in the catalog.
func (s *Store) EnsureSchema(ctx context.Context) error {
	if s.dialect.name == DialectPostgres {
		return coreerrs.WrapOperation(sqldialect.ExecPostgresDDL(ctx, s.db, []string{s.tableName}, s.postgresSchema()),
			"create saga schema")
	}
	if _, err := s.db.ExecContext(ctx, s.mysqlSchema()); err != nil {
		return coreerrs.WrapOperation(err, "create saga schema")
	}
	return nil
}

// postgresSchema lists the PostgreSQL DDL. Timestamps are Unix nanoseconds, 0
// for the zero time, so LeaseUntil keeps full precision.
func (s *Store) postgresSchema() []string {
	t := s.table
	return []string{
		`CREATE TABLE IF NOT EXISTS ` + t + ` (
  id            VARCHAR(255) COLLATE "C" PRIMARY KEY,
  definition    TEXT    NOT NULL DEFAULT '',
  status        TEXT    NOT NULL DEFAULT '',
  stage         BIGINT  NOT NULL DEFAULT 0,
  pending_steps TEXT    NOT NULL DEFAULT '',
  data          BYTEA   NOT NULL,
  steps         TEXT    NOT NULL DEFAULT '',
  created_at    BIGINT  NOT NULL DEFAULT 0,
  updated_at    BIGINT  NOT NULL DEFAULT 0,
  deadline      BIGINT  NOT NULL DEFAULT 0,
  lease_owner   TEXT    NOT NULL DEFAULT '',
  lease_until   BIGINT  NOT NULL DEFAULT 0,
  version       BIGINT  NOT NULL DEFAULT 0,
  last_error    TEXT    NOT NULL DEFAULT ''
)`,
		`CREATE INDEX IF NOT EXISTS ` + s.dialect.IndexName(s.tableName, "lease") + ` ON ` + t + ` (status, lease_until)`,
		`CREATE INDEX IF NOT EXISTS ` + s.dialect.IndexName(s.tableName, "deadline") + ` ON ` + t + ` (status, deadline)`,
	}
}

// mysqlSchema declares every string column as a binary type. The ID width is
// in bytes: four per character of [MaxIDLength], the most a utf8mb4 character
// takes. MySQL has no CREATE INDEX IF NOT EXISTS, so the indexes are declared
// inline.
func (s *Store) mysqlSchema() string {
	return `CREATE TABLE IF NOT EXISTS ` + s.table + ` (
  id            VARBINARY(1020) NOT NULL PRIMARY KEY,
  definition    MEDIUMBLOB      NOT NULL,
  status        VARBINARY(64)   NOT NULL DEFAULT '',
  stage         BIGINT          NOT NULL DEFAULT 0,
  pending_steps MEDIUMBLOB      NOT NULL,
  data          LONGBLOB        NOT NULL,
  steps         LONGBLOB        NOT NULL,
  created_at    BIGINT          NOT NULL DEFAULT 0,
  updated_at    BIGINT          NOT NULL DEFAULT 0,
  deadline      BIGINT          NOT NULL DEFAULT 0,
  lease_owner   MEDIUMBLOB      NOT NULL,
  lease_until   BIGINT          NOT NULL DEFAULT 0,
  version       BIGINT          NOT NULL DEFAULT 0,
  last_error    LONGBLOB        NOT NULL,
  INDEX ` + s.dialect.IndexName(s.tableName, "lease") + ` (status, lease_until),
  INDEX ` + s.dialect.IndexName(s.tableName, "deadline") + ` (status, deadline)
) ENGINE=InnoDB`
}
