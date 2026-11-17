// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldialect

import (
	"reflect"
	"slices"
)

// MySQLErrorNumber returns the uint16 Number field of the first error in
// err's error tree that has one, in depth-first order, or 0 if none does.
// This recognizes go-sql-driver/mysql's MySQLError without importing a driver.
func MySQLErrorNumber(err error) uint16 {
	n, _ := mysqlErrorNumber(err)
	return n
}

func mysqlErrorNumber(err error) (uint16, bool) {
	if err == nil {
		return 0, false
	}
	v := reflect.Indirect(reflect.ValueOf(err))
	if v.Kind() == reflect.Struct {
		if f := v.FieldByName("Number"); f.IsValid() && f.Kind() == reflect.Uint16 {
			return uint16(f.Uint()), true //nolint:gosec // read from a uint16
		}
	}
	switch e := err.(type) {
	case interface{ Unwrap() []error }:
		for _, child := range e.Unwrap() {
			if n, ok := mysqlErrorNumber(child); ok {
				return n, true
			}
		}
	case interface{ Unwrap() error }:
		return mysqlErrorNumber(e.Unwrap())
	}
	return 0, false
}

// IsDuplicateKey reports a PostgreSQL unique violation (SQLSTATE 23505) or a
// MySQL duplicate key (error number 1062), including wrapped and joined errors.
func IsDuplicateKey(err error) bool {
	const mysqlDuplicateKey = 1062 // MySQL ER_DUP_ENTRY
	return postgresUniqueViolation(err) || MySQLErrorNumber(err) == mysqlDuplicateKey
}

func postgresUniqueViolation(err error) bool {
	// Walk the whole tree: the first SQLSTATE in a joined tree may be another
	// server error, while a later branch carries the unique violation.
	if e, ok := err.(interface{ SQLState() string }); ok && e.SQLState() == "23505" {
		return true
	}
	switch e := err.(type) {
	case interface{ Unwrap() []error }:
		return slices.ContainsFunc(e.Unwrap(), postgresUniqueViolation)
	case interface{ Unwrap() error }:
		return postgresUniqueViolation(e.Unwrap())
	}
	return false
}
