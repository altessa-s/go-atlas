// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb

import (
	"context"

	"github.com/altessa-s/go-atlas/internal/sqldialect"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

// EnsureSchema creates the table and expiry index if they do not exist.
// PostgreSQL DDL is serialized under an advisory lock; MySQL declares the index
// inline. Run at startup or apply equivalent DDL through migrations. It does
// not join WithTx.
func (s *Storage) EnsureSchema(ctx context.Context) error {
	index := s.dialect.IndexName(s.name, "expires_key")
	if s.dialect.name == DialectPostgres {
		ddl := []string{
			`CREATE TABLE IF NOT EXISTS ` + s.table + ` (
  key        VARCHAR(255) COLLATE "C" PRIMARY KEY,
  val        BYTEA NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  revision   BIGINT NOT NULL DEFAULT 0
)`,
			`CREATE INDEX IF NOT EXISTS ` + index + ` ON ` + s.table + ` (expires_at, key)`,
		}
		return coreerrs.WrapOperation(sqldialect.ExecPostgresDDL(ctx, s.db, []string{s.name}, ddl), "create idempotency schema")
	}
	_, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS `+s.table+` (
  `+s.dialect.Ident("key")+` VARBINARY(1020) NOT NULL PRIMARY KEY,
  val        LONGBLOB NOT NULL,
  expires_at DATETIME(6) NOT NULL,
  revision   BIGINT NOT NULL DEFAULT 0,
  INDEX `+index+` (expires_at, `+s.dialect.Ident("key")+`)
) ENGINE=InnoDB`)
	return coreerrs.WrapOperation(err, "create idempotency schema")
}
