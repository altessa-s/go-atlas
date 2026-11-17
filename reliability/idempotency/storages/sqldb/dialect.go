// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb

import (
	"errors"
	"time"

	"github.com/altessa-s/go-atlas/internal/sqldialect"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

// Dialect selects the SQL flavor the storage speaks.
type Dialect string

const (
	// DialectPostgres targets PostgreSQL 12+.
	DialectPostgres Dialect = "postgres"
	// DialectMySQL targets MySQL 8.0+ and MariaDB 10.6+.
	DialectMySQL Dialect = "mysql"
)

// Storage validation errors.
var (
	// ErrUnsupportedDialect reports a dialect other than postgres or mysql.
	ErrUnsupportedDialect = sqldialect.ErrUnsupportedDialect
	// ErrInvalidTableName reports an invalid or oversized SQL identifier.
	ErrInvalidTableName = sqldialect.ErrInvalidTableName
	// ErrValueTooLong reports a key longer than MaxKeyLength characters.
	ErrValueTooLong = errors.New("sqldb: value exceeds its column length")
)

// MaxKeyLength bounds a storage key, including the Keeper's namespace, in characters.
const MaxKeyLength = 255

type dialect struct {
	sqldialect.Style
	name Dialect
	now  string
}

func dialectFor(d Dialect) (dialect, error) {
	switch d {
	case DialectPostgres:
		// Unlike now(), statement_timestamp() advances inside a business
		// transaction, matching MongoDB's per-operation server clock.
		return dialect{Style: sqldialect.Postgres, name: d, now: "statement_timestamp()"}, nil
	case DialectMySQL:
		return dialect{Style: sqldialect.MySQL, name: d, now: "UTC_TIMESTAMP(6)"}, nil
	default:
		return dialect{}, coreerrs.Wrapf(ErrUnsupportedDialect, "%q", d)
	}
}

func (d dialect) bind(q string) string { return d.Bind(q, 1) }

func (d dialect) expiresIn() string {
	if d.name == DialectPostgres {
		return "(" + d.now + " + make_interval(secs => ?))"
	}
	return "(" + d.now + " + INTERVAL ? MICROSECOND)"
}

func (d dialect) duration(ttl time.Duration) any {
	if d.name == DialectPostgres {
		return ttl.Seconds()
	}
	return ttl.Microseconds()
}

func (d dialect) keyArg(key string) any {
	if d.name == DialectPostgres {
		return key
	}
	return []byte(key)
}
