// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb

import (
	"context"

	corecontext "github.com/altessa-s/go-atlas/core/context"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// createSchema creates the events table and its indexes if they do not exist,
// bounded by the schema timeout. It is idempotent, like the MongoDB store's
// index creation.
func (s *Store) createSchema(base context.Context) error {
	ctx, cancel := corecontext.ApplyTimeout(base, s.schemaTimeout)
	defer cancel()
	for _, stmt := range s.schema() {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return coreerrs.WrapOperationWithContext(err, "create outbox schema", "table "+s.table)
		}
	}
	return nil
}

func (s *Store) schema() []string {
	t, d := s.table, s.dialect
	if d.name == DialectPostgres {
		return []string{
			`CREATE TABLE IF NOT EXISTS ` + t + ` (
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
)`,
			`CREATE INDEX IF NOT EXISTS ` + d.indexName(s.name, "fetch") + ` ON ` + t + ` (status, created_at, seq)`,
			`CREATE INDEX IF NOT EXISTS ` + d.indexName(s.name, "locked") + ` ON ` + t + ` (status, locked_on)`,
			`CREATE INDEX IF NOT EXISTS ` + d.indexName(s.name, "published") + ` ON ` + t + ` (status, published_at)`,
		}
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
  expires_at      DATETIME(6)    NULL,
  INDEX ` + d.indexName(s.name, "fetch") + ` (status, created_at, seq),
  INDEX ` + d.indexName(s.name, "locked") + ` (status, locked_on),
  INDEX ` + d.indexName(s.name, "published") + ` (status, published_at)
) ENGINE=InnoDB`,
	}
}
