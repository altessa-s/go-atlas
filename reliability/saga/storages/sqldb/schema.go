// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb

import (
	"context"
	"strings"

	"github.com/altessa-s/go-atlas/internal/sqldialect"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

// indexes defines every secondary index for both dialects and schema repair.
var indexes = [...]struct{ suffix, columns string }{
	{"lease", "status, lease_until"},
	{"deadline", "status, deadline"},
	{"terminal", "status, updated_at"},
}

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
		return coreerrs.WrapOperation(sqldialect.ExecPostgresDDL(ctx, s.db.DB, []string{s.tableName}, s.postgresSchema()),
			"create saga schema")
	}
	_, err := s.db.DB.ExecContext(ctx, s.mysqlSchema())
	return coreerrs.WrapOperation(err, "create saga schema")
}

// postgresSchema lists the PostgreSQL DDL. Timestamps are Unix nanoseconds, 0
// for the zero time, so LeaseUntil keeps full precision.
func (s *Store) postgresSchema() []string {
	t := s.table
	stmts := make([]string, 0, 1+len(indexes))
	stmts = append(stmts,
		`CREATE TABLE IF NOT EXISTS `+t+` (
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
)`)
	for _, ix := range indexes {
		stmts = append(stmts, `CREATE INDEX IF NOT EXISTS `+s.dialect.IndexName(s.tableName, ix.suffix)+` ON `+t+` (`+ix.columns+`)`)
	}
	return stmts
}

// mysqlSchema declares every string column as a binary type. The ID width is
// in bytes: four per character of [MaxIDLength], the most a utf8mb4 character
// takes. MySQL has no CREATE INDEX IF NOT EXISTS, so the indexes are declared
// inline.
func (s *Store) mysqlSchema() string {
	var inline strings.Builder
	for _, ix := range indexes {
		inline.WriteString(",\n  INDEX " + s.dialect.IndexName(s.tableName, ix.suffix) + " (" + ix.columns + ")")
	}
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
  last_error    LONGBLOB        NOT NULL` + inline.String() + `
) ENGINE=InnoDB`
}
