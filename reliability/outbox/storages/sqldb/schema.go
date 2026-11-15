// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb

import (
	"context"
	"errors"
	"strings"

	"github.com/altessa-s/go-atlas/internal/sqldialect"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

// indexes are the events table's secondary indexes: the suffix of each name
// (see [sqldialect.Style.IndexName]) and its columns. Every one leads with
// status, the column every statement of the store filters on.
var indexes = [...]struct{ suffix, columns string }{
	// The claim query, one status at a time, in its ORDER BY order; Stats.
	{"fetch", "status, created_at, seq"},
	// The unlock sweep.
	{"locked", "status, locked_on"},
	// Retention.
	{"published", "status, published_at"},
	// The expiry sweep.
	{"expires", "status, expires_at"},
}

// EnsureSchema creates the events table and its indexes if they do not exist,
// and adds to an existing table the indexes a newer version of the store
// introduced. It is idempotent and safe to run concurrently from several
// instances; call it once at startup, or apply the same DDL through your
// migration tool — [New] performs no I/O. ctx bounds the DDL.
//
// On PostgreSQL the DDL runs in one transaction under an advisory lock on the
// table name, so instances creating the same absent table at once take turns
// instead of colliding in the catalog. MySQL serializes concurrent CREATE
// TABLE IF NOT EXISTS itself; it has no CREATE INDEX IF NOT EXISTS, so the
// indexes a table lacks are read from information_schema and added one by one.
//
// Adding an index to a large table blocks writes on PostgreSQL while it
// builds; create it beforehand with CREATE INDEX CONCURRENTLY to avoid that.
func (s *Store) EnsureSchema(ctx context.Context) error {
	if s.dialect.name == DialectPostgres {
		return coreerrs.WrapOperationWithContext(sqldialect.ExecPostgresDDL(ctx, s.db, []string{s.name}, s.ddl()),
			"create outbox schema", "table "+s.table)
	}
	for _, stmt := range s.ddl() {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return coreerrs.WrapOperationWithContext(err, "create outbox schema", "table "+s.table)
		}
	}
	return coreerrs.WrapOperationWithContext(s.addMissingIndexes(ctx), "create outbox indexes", "table "+s.table)
}

// addMissingIndexes brings a MySQL table created by an earlier version of the
// store up to the current index set.
func (s *Store) addMissingIndexes(ctx context.Context) error {
	var schema any // NULL selects the connection's default database.
	if s.schema != "" {
		schema = s.schema
	}
	rows, err := s.db.QueryContext(ctx,
		"SELECT DISTINCT index_name FROM information_schema.statistics WHERE table_schema = COALESCE(?, DATABASE()) AND table_name = ?",
		schema, s.name)
	if err != nil {
		return err
	}
	existing := map[string]struct{}{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			_ = rows.Close()
			return err
		}
		existing[name] = struct{}{}
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return err
	}

	for _, ix := range indexes {
		name := s.dialect.IndexName(s.name, ix.suffix)
		if _, ok := existing[strings.Trim(name, string(s.dialect.Quote))]; ok {
			continue
		}
		_, err := s.db.ExecContext(ctx, "ALTER TABLE "+s.table+" ADD INDEX "+name+" ("+ix.columns+")")
		// Another instance adding the same index at the same time wins the race.
		if err != nil && mysqlErrorNumber(err) != mysqlDupIndexName {
			return err
		}
	}
	return nil
}

// ddl returns the statements that create the events table and its indexes.
func (s *Store) ddl() []string {
	t, d := s.table, s.dialect
	if d.name == DialectPostgres {
		stmts := make([]string, 0, 1+len(indexes))
		stmts = append(stmts,
			`CREATE TABLE IF NOT EXISTS `+t+` (
  id              VARCHAR(255) COLLATE "C" PRIMARY KEY,
  seq             BIGINT       GENERATED ALWAYS AS IDENTITY,
  topic           BYTEA        NOT NULL,
  payload         BYTEA        NULL,
  status          VARCHAR(32)  NOT NULL,
  created_at      TIMESTAMPTZ  NOT NULL,
  attempts        BIGINT       NOT NULL DEFAULT 0,
  error           BYTEA        NULL,
  last_attempt_on TIMESTAMPTZ  NULL,
  locked_on       TIMESTAMPTZ  NULL,
  lock_token      VARCHAR(64)  NULL,
  next_attempt_at TIMESTAMPTZ  NULL,
  published_at    TIMESTAMPTZ  NULL,
  expires_at      TIMESTAMPTZ  NULL
)`)
		for _, ix := range indexes {
			stmts = append(stmts, `CREATE INDEX IF NOT EXISTS `+d.IndexName(s.name, ix.suffix)+` ON `+t+` (`+ix.columns+`)`)
		}
		return stmts
	}
	// topic and error are arbitrary Go strings — a handler error may contain
	// NUL, which PostgreSQL TEXT rejects — so they are stored as bytes.
	//
	// seq records insertion order: Save stamps a whole batch with one
	// CreatedAt, and compaction needs the last-saved event of a key to sort
	// last, so seq — not the random ID — breaks CreatedAt ties.
	//
	// Binary string types compare byte-wise with no padding or case folding and
	// never inherit the database's default character set. MySQL has no CREATE
	// INDEX IF NOT EXISTS, so the indexes are declared inline.
	var inline strings.Builder
	for _, ix := range indexes {
		inline.WriteString(",\n  INDEX " + d.IndexName(s.name, ix.suffix) + " (" + ix.columns + ")")
	}
	return []string{
		`CREATE TABLE IF NOT EXISTS ` + t + ` (
  id              VARBINARY(255) NOT NULL PRIMARY KEY,
  seq             BIGINT         NOT NULL AUTO_INCREMENT UNIQUE,
  topic           LONGBLOB       NOT NULL,
  payload         LONGBLOB       NULL,
  status          VARCHAR(32)    NOT NULL,
  created_at      DATETIME(6)    NOT NULL,
  attempts        BIGINT         NOT NULL DEFAULT 0,
  error           LONGBLOB       NULL,
  last_attempt_on DATETIME(6)    NULL,
  locked_on       DATETIME(6)    NULL,
  lock_token      VARBINARY(64)  NULL,
  next_attempt_at DATETIME(6)    NULL,
  published_at    DATETIME(6)    NULL,
  expires_at      DATETIME(6)    NULL` + inline.String() + `
) ENGINE=InnoDB`,
	}
}
