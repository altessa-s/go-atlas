// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb

import (
	"context"
	"strings"

	"github.com/altessa-s/go-atlas/internal/sqldialect"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// EnsureSchema creates the tasks and history tables and their indexes if they
// do not exist, and adds the run-lease and occurrence columns (run_lease_until,
// run_lease_id, run_at) to a tasks table created by an earlier release. It is
// idempotent and safe to run concurrently from several instances; call it once
// at startup (or apply the same DDL through your migration tool).
//
// On MySQL/MariaDB every string column is a binary type (VARBINARY, MEDIUMBLOB,
// LONGBLOB), so IDs and run-ownership fences compare exactly — byte-wise, no
// trailing space padding, no case folding — on every supported server and
// whatever the database defaults are. Tables created by an earlier release
// with NO PAD binary collations compare exactly too and are left as they are.
//
// On PostgreSQL the DDL runs in one transaction under advisory locks on the
// table names, so instances creating the same absent tables at once take
// turns instead of colliding in the catalog. MySQL serializes concurrent
// CREATE TABLE IF NOT EXISTS itself.
func (s *Storage) EnsureSchema(ctx context.Context) error {
	if s.dialect.name == DialectPostgres {
		return coreerrs.WrapOperation(sqldialect.ExecPostgresDDL(ctx, s.db, []string{s.tasksName, s.historyName}, s.postgresSchema()),
			"create scheduler schema")
	}
	for _, stmt := range s.mysqlSchema() {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return coreerrs.WrapOperation(err, "create scheduler schema")
		}
	}
	return s.mysqlAddColumns(ctx)
}

// addedColumn is a tasks column introduced after the first release of the
// schema; EnsureSchema adds it to an existing table.
type addedColumn struct {
	name     string
	postgres string // type and constraints on PostgreSQL
	mysql    string // type and constraints on MySQL
}

// addedColumns lists the tasks columns EnsureSchema adds to an existing table,
// in the order they are added. Each matches its CREATE TABLE definition.
var addedColumns = []addedColumn{
	{"run_lease_until", "BIGINT NOT NULL DEFAULT 0", "BIGINT NOT NULL DEFAULT 0"},
	{"run_lease_id", "TEXT NOT NULL DEFAULT ''", "VARBINARY(1020) NOT NULL DEFAULT ''"},
	{"run_at", "BIGINT NOT NULL DEFAULT 0", "BIGINT NOT NULL DEFAULT 0"},
}

// mysqlAddColumns adds the columns of addedColumns missing from the tasks
// table. MySQL has no ADD COLUMN IF NOT EXISTS (MariaDB does, MySQL 8 does
// not), so the columns are read from information_schema first; an ALTER that
// fails because a concurrent EnsureSchema added the column meanwhile is
// tolerated by checking again.
func (s *Storage) mysqlAddColumns(ctx context.Context) error {
	existing, err := s.mysqlTaskColumns(ctx)
	if err != nil {
		return err
	}
	for _, c := range addedColumns {
		if existing[c.name] {
			continue
		}
		if _, err := s.db.ExecContext(ctx, "ALTER TABLE "+s.tasksTable+" ADD COLUMN "+c.name+" "+c.mysql); err != nil {
			if now, checkErr := s.mysqlTaskColumns(ctx); checkErr == nil && now[c.name] {
				continue
			}
			return coreerrs.WrapOperation(err, "upgrade scheduler schema")
		}
	}
	return nil
}

// mysqlTaskColumns returns the column names of the tasks table, looked up in
// its schema qualifier or, when unqualified, the connection's database.
func (s *Storage) mysqlTaskColumns(ctx context.Context) (map[string]bool, error) {
	const query = "SELECT COLUMN_NAME FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = COALESCE(?, DATABASE()) AND TABLE_NAME = ?"
	var schema any // NULL selects the connection's database
	if s.tasksSchema != "" {
		schema = s.tasksSchema
	}
	rows, err := s.db.QueryContext(ctx, query, schema, s.tasksName)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "read scheduler schema")
	}
	defer func() { _ = rows.Close() }()
	cols := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, coreerrs.WrapOperation(err, "read scheduler schema")
		}
		cols[strings.ToLower(name)] = true
	}
	if err := rows.Err(); err != nil {
		return nil, coreerrs.WrapOperation(err, "read scheduler schema")
	}
	return cols, nil
}

// postgresSchema lists the PostgreSQL DDL with every history statement before
// every tasks statement. EnsureSchema runs it in one transaction, which holds
// each table lock until commit (CREATE INDEX IF NOT EXISTS locks an existing
// table too), so it must lock the tables in DeleteTask's order — history, then
// tasks — or the two can deadlock.
func (s *Storage) postgresSchema() []string {
	tasks, history := s.tasksTable, s.historyTable
	return []string{
		`CREATE TABLE IF NOT EXISTS ` + history + ` (
  id          VARCHAR(255) COLLATE "C" PRIMARY KEY,
  task_id     VARCHAR(255) COLLATE "C" NOT NULL,
  run_id      TEXT    NOT NULL DEFAULT '',
  error       TEXT    NOT NULL DEFAULT '',
  started_at  BIGINT  NOT NULL,
  ended_at    BIGINT  NOT NULL,
  duration_ms BIGINT  NOT NULL DEFAULT 0,
  success     BOOLEAN NOT NULL
)`,
		`CREATE INDEX IF NOT EXISTS ` + s.dialect.IndexName(s.historyName, "task") + ` ON ` + history + ` (task_id, started_at DESC, id DESC)`,
		`CREATE INDEX IF NOT EXISTS ` + s.dialect.IndexName(s.historyName, "ended") + ` ON ` + history + ` (ended_at)`,
		`CREATE TABLE IF NOT EXISTS ` + tasks + ` (
  id              VARCHAR(255) COLLATE "C" PRIMARY KEY,
  description     TEXT    NOT NULL DEFAULT '',
  status          INTEGER NOT NULL,
  priority        INTEGER NOT NULL DEFAULT 0,
  schedule        TEXT    NOT NULL DEFAULT '',
  last_run_at     BIGINT  NOT NULL DEFAULT 0,
  next_run_at     BIGINT  NOT NULL DEFAULT 0,
  last_run_id     TEXT    NOT NULL DEFAULT '',
  run_started_at  BIGINT  NOT NULL DEFAULT 0,
  run_lease_until BIGINT  NOT NULL DEFAULT 0,
  run_lease_id    TEXT    NOT NULL DEFAULT '',
  run_at          BIGINT  NOT NULL DEFAULT 0,
  failures        INTEGER NOT NULL DEFAULT 0,
  skip_next_run   BOOLEAN NOT NULL DEFAULT FALSE,
  disable_history BOOLEAN NOT NULL DEFAULT FALSE,
  unmanaged       BOOLEAN NOT NULL DEFAULT FALSE,
  one_shot        BOOLEAN NOT NULL DEFAULT FALSE,
  meta            TEXT    NOT NULL DEFAULT '{}',
  created_at      BIGINT  NOT NULL DEFAULT 0,
  updated_at      BIGINT  NOT NULL DEFAULT 0,
  revision        BIGINT  NOT NULL DEFAULT 0
)`,
		`CREATE INDEX IF NOT EXISTS ` + s.dialect.IndexName(s.tasksName, "due") + ` ON ` + tasks + ` (status, next_run_at)`,
		s.postgresAddColumns(),
	}
}

// postgresAddColumns renders one ALTER TABLE adding every column of
// addedColumns that the tasks table lacks; ADD COLUMN IF NOT EXISTS makes it a
// no-op on a current table, and ALTER TABLE's lock serializes concurrent runs.
func (s *Storage) postgresAddColumns() string {
	adds := make([]string, len(addedColumns))
	for i, c := range addedColumns {
		adds[i] = "ADD COLUMN IF NOT EXISTS " + c.name + " " + c.postgres
	}
	return "ALTER TABLE " + s.tasksTable + " " + strings.Join(adds, ", ")
}

// mysqlSchema declares every string column as a binary type. Widths are in
// bytes: four per character of [MaxIDLength] and [MaxScheduleLength], the most
// a utf8mb4 character takes, so any value within the character limits fits.
// MySQL has no CREATE INDEX IF NOT EXISTS, so the indexes are declared inline.
func (s *Storage) mysqlSchema() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS ` + s.tasksTable + ` (
  id              VARBINARY(1020) NOT NULL PRIMARY KEY,
  description     MEDIUMBLOB      NOT NULL,
  status          INT             NOT NULL,
  priority        INT             NOT NULL DEFAULT 0,
  schedule        VARBINARY(4096) NOT NULL DEFAULT '',
  last_run_at     BIGINT          NOT NULL DEFAULT 0,
  next_run_at     BIGINT          NOT NULL DEFAULT 0,
  last_run_id     VARBINARY(1020) NOT NULL DEFAULT '',
  run_started_at  BIGINT          NOT NULL DEFAULT 0,
  run_lease_until BIGINT          NOT NULL DEFAULT 0,
  run_lease_id    VARBINARY(1020) NOT NULL DEFAULT '',
  run_at          BIGINT          NOT NULL DEFAULT 0,
  failures        INT             NOT NULL DEFAULT 0,
  skip_next_run   BOOLEAN         NOT NULL DEFAULT FALSE,
  disable_history BOOLEAN         NOT NULL DEFAULT FALSE,
  unmanaged       BOOLEAN         NOT NULL DEFAULT FALSE,
  one_shot        BOOLEAN         NOT NULL DEFAULT FALSE,
  meta            MEDIUMBLOB      NOT NULL,
  created_at      BIGINT          NOT NULL DEFAULT 0,
  updated_at      BIGINT          NOT NULL DEFAULT 0,
  revision        BIGINT          NOT NULL DEFAULT 0,
  INDEX ` + s.dialect.IndexName(s.tasksName, "due") + ` (status, next_run_at)
) ENGINE=InnoDB`,
		`CREATE TABLE IF NOT EXISTS ` + s.historyTable + ` (
  id          VARBINARY(1020) NOT NULL PRIMARY KEY,
  task_id     VARBINARY(1020) NOT NULL,
  run_id      VARBINARY(1020) NOT NULL DEFAULT '',
  error       LONGBLOB        NOT NULL,
  started_at  BIGINT          NOT NULL,
  ended_at    BIGINT          NOT NULL,
  duration_ms BIGINT          NOT NULL DEFAULT 0,
  success     BOOLEAN         NOT NULL,
  INDEX ` + s.dialect.IndexName(s.historyName, "task") + ` (task_id, started_at DESC, id DESC),
  INDEX ` + s.dialect.IndexName(s.historyName, "ended") + ` (ended_at)
) ENGINE=InnoDB`,
	}
}
