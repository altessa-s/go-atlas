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

// indexes are the items table's secondary indexes: the suffix of each name
// (see [sqldialect.Style.IndexName]) and its columns. Each leads with state,
// the column every scan filters on, and ends in the sort its query needs.
var indexes = [...]struct{ suffix, columns string }{
	// ListDue and WakeDue.
	{"due", "state, next_attempt_at, id"},
	// ListByState and Requeue.
	{"state", "state, since, id"},
	// PurgeExpired.
	{"resolved", "state, resolved_at"},
}

// EnsureSchema creates the items table and its indexes if they do not exist.
// It is idempotent and safe to run concurrently from several instances; call
// it once at startup, or apply the same DDL through your migration tool —
// [New] performs no I/O. ctx bounds the DDL.
//
// On MySQL/MariaDB every string column is a binary type (VARBINARY,
// LONGBLOB), so keys compare exactly — byte-wise, no trailing space padding,
// no case folding — whatever the database defaults are; on PostgreSQL the key
// uses the "C" collation for the same order. On PostgreSQL the DDL runs in one
// transaction under an advisory lock on the table name, so instances creating
// the same absent table at once take turns instead of colliding in the
// catalog.
func (s *Store[P]) EnsureSchema(ctx context.Context) error {
	if s.dialect.name == DialectPostgres {
		return coreerrs.WrapOperationWithContext(sqldialect.ExecPostgresDDL(ctx, s.db, []string{s.tableName}, s.postgresSchema()),
			"create retry queue schema", "table "+s.table)
	}
	if _, err := s.db.ExecContext(ctx, s.mysqlSchema()); err != nil {
		return coreerrs.WrapOperationWithContext(err, "create retry queue schema", "table "+s.table)
	}
	indexesToEnsure := make([]sqldialect.MySQLIndex, 0, len(indexes))
	for _, ix := range indexes {
		indexesToEnsure = append(indexesToEnsure, sqldialect.MySQLIndex{
			Name: s.dialect.IndexName(s.tableName, ix.suffix), Columns: ix.columns,
		})
	}
	return coreerrs.WrapOperationWithContext(sqldialect.AddMissingMySQLIndexes(ctx, s.db, s.qualifier,
		s.tableName, indexesToEnsure), "create retry queue indexes", "table "+s.table)
}

// postgresSchema lists the PostgreSQL DDL. Instants are Unix nanoseconds, 0
// for the zero time; the flag reason is bytes because TEXT rejects NUL.
func (s *Store[P]) postgresSchema() []string {
	stmts := make([]string, 0, 1+len(indexes))
	stmts = append(stmts, `CREATE TABLE IF NOT EXISTS `+s.table+` (
  id              VARCHAR(255) COLLATE "C" PRIMARY KEY,
  generation      BIGINT       NOT NULL DEFAULT 0,
  state           VARCHAR(16)  COLLATE "C" NOT NULL,
  since           BIGINT       NOT NULL DEFAULT 0,
  attempts        BIGINT       NOT NULL DEFAULT 0,
  next_attempt_at BIGINT       NOT NULL DEFAULT 0,
  expired_at      BIGINT       NOT NULL DEFAULT 0,
  resolved_at     BIGINT       NOT NULL DEFAULT 0,
  flagged         BOOLEAN      NOT NULL DEFAULT FALSE,
  flag_reason     BYTEA        NOT NULL DEFAULT '',
  payload         BYTEA        NOT NULL
)`)
	for _, ix := range indexes {
		stmts = append(stmts, `CREATE INDEX IF NOT EXISTS `+s.dialect.IndexName(s.tableName, ix.suffix)+` ON `+s.table+` (`+ix.columns+`)`)
	}
	return stmts
}

// mysqlSchema declares every string column as a binary type. The key width is
// in bytes: four per character of [MaxKeyLength], the most a utf8mb4
// character takes. MySQL has no CREATE INDEX IF NOT EXISTS, so the indexes
// are declared inline.
func (s *Store[P]) mysqlSchema() string {
	var inline strings.Builder
	for _, ix := range indexes {
		inline.WriteString(",\n  INDEX " + s.dialect.IndexName(s.tableName, ix.suffix) + " (" + ix.columns + ")")
	}
	return `CREATE TABLE IF NOT EXISTS ` + s.table + ` (
  id              VARBINARY(1020) NOT NULL PRIMARY KEY,
  generation      BIGINT          NOT NULL DEFAULT 0,
  state           VARBINARY(16)   NOT NULL,
  since           BIGINT          NOT NULL DEFAULT 0,
  attempts        BIGINT          NOT NULL DEFAULT 0,
  next_attempt_at BIGINT          NOT NULL DEFAULT 0,
  expired_at      BIGINT          NOT NULL DEFAULT 0,
  resolved_at     BIGINT          NOT NULL DEFAULT 0,
  flagged         BOOLEAN         NOT NULL DEFAULT FALSE,
  flag_reason     LONGBLOB        NOT NULL,
  payload         LONGBLOB        NOT NULL` + inline.String() + `
) ENGINE=InnoDB`
}
