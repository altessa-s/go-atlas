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

// Columns of the events table that the schema, its indexes and the query
// filters all refer to.
const (
	colTimestamp    = "ts_ms"
	colActorID      = "actor_id"
	colRequestID    = "request_id"
	colResourceType = "resource_type"
	colResourceID   = "resource_id"
	colTraceID      = "trace_id"
)

// filterColumns are the queryable event fields, stored beside the payload.
var filterColumns = []string{"event_type", "action", colActorID, "actor_type", colResourceType, colResourceID, "status",
	colRequestID, colTraceID}

// index is one secondary index of the events table.
type index struct {
	suffix  string
	columns []string
}

// indexes back the time-ordered scan and the common filters; each ends in the
// sort key so a filtered page reads in order.
var indexes = []index{
	{"ts", []string{colTimestamp, "id"}},
	{"actor", []string{colActorID, colTimestamp, "id"}},
	{"resource", []string{colResourceType, colResourceID, colTimestamp, "id"}},
	{"request", []string{colRequestID}},
	{"trace", []string{colTraceID}},
}

// EnsureSchema creates the events table and its indexes if they do not exist.
// It is idempotent and
// safe to run concurrently from several instances; call it once at startup
// (or apply the same DDL through your migration tool).
//
// On MySQL/MariaDB every string column is a binary type, so IDs and filters
// compare byte-wise whatever the database defaults are. On PostgreSQL the DDL
// runs in one transaction under an advisory lock on the table name.
func (s *Storage) EnsureSchema(ctx context.Context) error {
	if s.dialect.name == DialectPostgres {
		return coreerrs.WrapOperation(sqldialect.ExecPostgresDDL(ctx, s.db, []string{s.tableName}, s.postgresSchema()),
			"create audit schema")
	}
	_, err := s.db.ExecContext(ctx, s.mysqlSchema())
	return coreerrs.WrapOperation(err, "create audit schema")
}

// postgresSchema lists the PostgreSQL DDL. The ID and filter columns use the
// "C" collation, so ordering and equality are byte-wise like the cursor's.
func (s *Storage) postgresSchema() []string {
	var b strings.Builder
	b.WriteString("CREATE TABLE IF NOT EXISTS " + s.table + " (\n  id VARCHAR(255) COLLATE \"C\" PRIMARY KEY,\n  ts_ms BIGINT NOT NULL,\n")
	for _, c := range filterColumns {
		b.WriteString("  " + c + " TEXT COLLATE \"C\" NOT NULL DEFAULT '',\n")
	}
	b.WriteString("  payload TEXT NOT NULL\n)")
	stmts := make([]string, 0, 1+len(indexes))
	stmts = append(stmts, b.String())
	for _, ix := range indexes {
		stmts = append(stmts, "CREATE INDEX IF NOT EXISTS "+s.dialect.IndexName(s.tableName, ix.suffix)+" ON "+s.table+
			" ("+strings.Join(ix.columns, ", ")+")")
	}
	return stmts
}

// mysqlSchema declares every string column as a binary type: the ID as
// VARBINARY (four bytes per character of [MaxIDLength]), filter values as
// VARBINARY of [MaxFilterBytes]. MySQL has no CREATE INDEX IF NOT EXISTS, so
// the indexes are declared inline.
func (s *Storage) mysqlSchema() string {
	var b strings.Builder
	b.WriteString("CREATE TABLE IF NOT EXISTS " + s.table + " (\n  id VARBINARY(1020) NOT NULL PRIMARY KEY,\n  ts_ms BIGINT NOT NULL,\n")
	for _, c := range filterColumns {
		b.WriteString("  " + c + " VARBINARY(255) NOT NULL DEFAULT '',\n")
	}
	b.WriteString("  payload LONGBLOB NOT NULL")
	for _, ix := range indexes {
		b.WriteString(",\n  INDEX " + s.dialect.IndexName(s.tableName, ix.suffix) + " (" + strings.Join(ix.columns, ", ") + ")")
	}
	b.WriteString("\n) ENGINE=InnoDB")
	return b.String()
}
