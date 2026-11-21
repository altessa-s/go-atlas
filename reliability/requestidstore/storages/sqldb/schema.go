// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb

import (
	"context"

	"github.com/altessa-s/go-atlas/internal/sqldialect"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

// EnsureSchema creates the entries table and expiry index. It is safe to call repeatedly.
func (s *Storage) EnsureSchema(ctx context.Context) error {
	index := s.style.IndexName(s.name, "expires_key")
	if s.style == sqldialect.Postgres {
		ddl := []string{
			"CREATE TABLE IF NOT EXISTS " + s.table + ` (
    request_key VARCHAR(64) COLLATE "C" PRIMARY KEY,
    state SMALLINT NOT NULL,
    token BYTEA NOT NULL,
    fingerprint BYTEA,
    response BYTEA,
    failure BYTEA,
    response_omitted BOOLEAN NOT NULL,
    expires_at BIGINT NOT NULL,
    revision BIGINT NOT NULL DEFAULT 0
)`,
			"CREATE INDEX IF NOT EXISTS " + index + " ON " + s.table + " (expires_at, request_key)",
		}
		return coreerrs.WrapOperation(sqldialect.ExecPostgresDDL(ctx, s.db, []string{s.name}, ddl), "create request ID schema")
	}
	_, err := s.db.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS "+s.table+` (
    request_key VARBINARY(64) NOT NULL PRIMARY KEY,
    state SMALLINT NOT NULL,
    token LONGBLOB NOT NULL,
    fingerprint LONGBLOB,
    response LONGBLOB,
    failure LONGBLOB,
    response_omitted BOOLEAN NOT NULL,
    expires_at BIGINT NOT NULL,
    revision BIGINT NOT NULL DEFAULT 0,
    INDEX `+index+` (expires_at, request_key)
) ENGINE=InnoDB`)
	return coreerrs.WrapOperation(err, "create request ID schema")
}
