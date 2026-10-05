// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb

import (
	"context"
	"fmt"
	"hash/fnv"
	"strconv"
	"strings"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// Minimum server versions for the MySQL dialect. MySQL needs 8.0.17 for the
// utf8mb4_0900_bin collation; MariaDB 10.6 is the documented floor.
var (
	minMySQL   = [3]int{8, 0, 17}
	minMariaDB = [3]int{10, 6, 0}
)

// EnsureSchema creates the tasks and history tables and their indexes if they
// do not exist, and adds the run-lease and occurrence columns (run_lease_until,
// run_lease_id, run_at) to a tasks table created by an earlier release. It is
// idempotent and safe to run concurrently from several instances; call it once
// at startup (or apply the same DDL through your migration tool).
//
// On MySQL/MariaDB every string column gets an explicit utf8mb4 character set
// and a NO PAD binary collation (utf8mb4_0900_bin on MySQL, utf8mb4_nopad_bin
// on MariaDB), so IDs and run-ownership fences compare exactly — no trailing
// space padding, no case folding — whatever the database defaults are. The
// server is probed once to pick the collation; older servers are rejected
// with [ErrUnsupportedVersion].
func (s *Storage) EnsureSchema(ctx context.Context) error {
	var (
		ddl       []string
		collation string
	)
	switch s.dialect.name {
	case DialectPostgres:
		ddl = s.postgresSchema()
	case DialectMySQL:
		var err error
		if collation, err = s.mysqlCollation(ctx); err != nil {
			return err
		}
		ddl = s.mysqlSchema(collation)
	}
	for _, stmt := range ddl {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return coreerrs.WrapOperation(err, "create scheduler schema")
		}
	}
	if s.dialect.name == DialectMySQL {
		return s.mysqlAddColumns(ctx, collation)
	}
	return nil
}

// addedColumn is a tasks column introduced after the first release of the
// schema; EnsureSchema adds it to an existing table.
type addedColumn struct {
	name     string
	postgres string // type and constraints on PostgreSQL
	mysql    string // type and constraints on MySQL; %s is the string collation clause
}

// addedColumns lists the tasks columns EnsureSchema adds to an existing table,
// in the order they are added. Each matches its CREATE TABLE definition.
var addedColumns = []addedColumn{
	{"run_lease_until", "BIGINT NOT NULL DEFAULT 0", "BIGINT NOT NULL DEFAULT 0"},
	{"run_lease_id", "TEXT NOT NULL DEFAULT ''", "VARCHAR(255) %s NOT NULL DEFAULT ''"},
	{"run_at", "BIGINT NOT NULL DEFAULT 0", "BIGINT NOT NULL DEFAULT 0"},
}

// mysqlAddColumns adds the columns of addedColumns missing from the tasks
// table. MySQL has no ADD COLUMN IF NOT EXISTS (MariaDB does, MySQL 8 does
// not), so the columns are read from information_schema first; an ALTER that
// fails because a concurrent EnsureSchema added the column meanwhile is
// tolerated by checking again.
func (s *Storage) mysqlAddColumns(ctx context.Context, collation string) error {
	existing, err := s.mysqlTaskColumns(ctx)
	if err != nil {
		return err
	}
	for _, c := range addedColumns {
		if existing[c.name] {
			continue
		}
		def := c.mysql
		if strings.Contains(def, "%s") {
			def = fmt.Sprintf(def, "CHARACTER SET utf8mb4 COLLATE "+collation)
		}
		if _, err := s.db.ExecContext(ctx, "ALTER TABLE "+s.tasksTable+" ADD COLUMN "+c.name+" "+def); err != nil {
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

func (s *Storage) postgresSchema() []string {
	tasks, history := s.tasksTable, s.historyTable
	return []string{
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
		`CREATE INDEX IF NOT EXISTS ` + s.indexName(s.tasksName, "due") + ` ON ` + tasks + ` (status, next_run_at)`,
		s.postgresAddColumns(),
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
		`CREATE INDEX IF NOT EXISTS ` + s.indexName(s.historyName, "task") + ` ON ` + history + ` (task_id, started_at DESC, id DESC)`,
		`CREATE INDEX IF NOT EXISTS ` + s.indexName(s.historyName, "ended") + ` ON ` + history + ` (ended_at)`,
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

func (s *Storage) mysqlSchema(collation string) []string {
	text := func(typ string) string { return typ + " CHARACTER SET utf8mb4 COLLATE " + collation }
	return []string{
		`CREATE TABLE IF NOT EXISTS ` + s.tasksTable + ` (
  id              ` + text("VARCHAR(255)") + ` NOT NULL PRIMARY KEY,
  description     ` + text("MEDIUMTEXT") + ` NOT NULL,
  status          INT     NOT NULL,
  priority        INT     NOT NULL DEFAULT 0,
  schedule        ` + text("VARCHAR(1024)") + ` NOT NULL DEFAULT '',
  last_run_at     BIGINT  NOT NULL DEFAULT 0,
  next_run_at     BIGINT  NOT NULL DEFAULT 0,
  last_run_id     ` + text("VARCHAR(255)") + ` NOT NULL DEFAULT '',
  run_started_at  BIGINT  NOT NULL DEFAULT 0,
  run_lease_until BIGINT  NOT NULL DEFAULT 0,
  run_lease_id    ` + text("VARCHAR(255)") + ` NOT NULL DEFAULT '',
  run_at          BIGINT  NOT NULL DEFAULT 0,
  failures        INT     NOT NULL DEFAULT 0,
  skip_next_run   BOOLEAN NOT NULL DEFAULT FALSE,
  disable_history BOOLEAN NOT NULL DEFAULT FALSE,
  unmanaged       BOOLEAN NOT NULL DEFAULT FALSE,
  one_shot        BOOLEAN NOT NULL DEFAULT FALSE,
  meta            ` + text("MEDIUMTEXT") + ` NOT NULL,
  created_at      BIGINT  NOT NULL DEFAULT 0,
  updated_at      BIGINT  NOT NULL DEFAULT 0,
  revision        BIGINT  NOT NULL DEFAULT 0,
  INDEX ` + s.indexName(s.tasksName, "due") + ` (status, next_run_at)
) ENGINE=InnoDB`,
		`CREATE TABLE IF NOT EXISTS ` + s.historyTable + ` (
  id          ` + text("VARCHAR(255)") + ` NOT NULL PRIMARY KEY,
  task_id     ` + text("VARCHAR(255)") + ` NOT NULL,
  run_id      ` + text("VARCHAR(255)") + ` NOT NULL DEFAULT '',
  error       ` + text("LONGTEXT") + ` NOT NULL,
  started_at  BIGINT  NOT NULL,
  ended_at    BIGINT  NOT NULL,
  duration_ms BIGINT  NOT NULL DEFAULT 0,
  success     BOOLEAN NOT NULL,
  INDEX ` + s.indexName(s.historyName, "task") + ` (task_id, started_at DESC, id DESC),
  INDEX ` + s.indexName(s.historyName, "ended") + ` (ended_at)
) ENGINE=InnoDB`,
	}
}

// maxIdentLen is the shorter of PostgreSQL's (63) and MySQL's (64) identifier
// limits. PostgreSQL silently truncates longer names, which could make two
// tables' indexes collide; MySQL rejects them.
const maxIdentLen = 63

// indexName derives a quoted index name from the unqualified table name, so
// storages over different tables never collide on index names. A name that
// would exceed maxIdentLen keeps a readable prefix and ends in a hash of the
// full table name.
func (s *Storage) indexName(table, suffix string) string {
	q := string(s.dialect.quote)
	name := table + "_" + suffix + "_idx"
	if len(name) > maxIdentLen {
		h := fnv.New32a()
		_, _ = h.Write([]byte(table))
		tail := fmt.Sprintf("_%08x_%s_idx", h.Sum32(), suffix)
		name = table[:maxIdentLen-len(tail)] + tail
	}
	return q + name + q
}

// mysqlCollation probes the server and returns the NO PAD binary collation it
// supports.
func (s *Storage) mysqlCollation(ctx context.Context) (string, error) {
	var version string
	if err := s.db.QueryRowContext(ctx, "SELECT VERSION()").Scan(&version); err != nil {
		return "", coreerrs.WrapOperation(err, "detect database version")
	}
	return collationFor(version)
}

// collationFor maps a VERSION() string to the NO PAD binary collation.
func collationFor(version string) (string, error) {
	mariaDB := strings.Contains(strings.ToLower(version), "mariadb")
	v := parseVersion(version)
	switch {
	case mariaDB && !versionAtLeast(v, minMariaDB):
		return "", fmt.Errorf("%w: MariaDB %s, need 10.6+", ErrUnsupportedVersion, version)
	case mariaDB:
		return "utf8mb4_nopad_bin", nil
	case !versionAtLeast(v, minMySQL):
		return "", fmt.Errorf("%w: MySQL %s, need 8.0.17+", ErrUnsupportedVersion, version)
	default:
		return "utf8mb4_0900_bin", nil
	}
}

// parseVersion extracts major.minor.patch from strings such as "8.4.2",
// "8.0.36-log" or "11.4.2-MariaDB-ubu2404"; missing parts are zero.
func parseVersion(s string) [3]int {
	var v [3]int
	head, _, _ := strings.Cut(s, "-")
	for i, part := range strings.SplitN(head, ".", len(v)) {
		digits := part
		if j := strings.IndexFunc(part, func(r rune) bool { return r < '0' || r > '9' }); j >= 0 {
			digits = part[:j]
		}
		v[i], _ = strconv.Atoi(digits)
	}
	return v
}

func versionAtLeast(v, floor [3]int) bool {
	for i := range v {
		if v[i] != floor[i] {
			return v[i] > floor[i]
		}
	}
	return true
}
