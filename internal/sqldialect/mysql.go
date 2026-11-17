// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldialect

import (
	"errors"
	"reflect"
)

// MySQLErrorNumber returns the uint16 Number field of the first error in
// err's unwrap chain that has one, or 0 if none does. This recognizes
// go-sql-driver/mysql's MySQLError without importing a SQL driver.
func MySQLErrorNumber(err error) uint16 {
	for ; err != nil; err = errors.Unwrap(err) {
		v := reflect.Indirect(reflect.ValueOf(err))
		if v.Kind() != reflect.Struct {
			continue
		}
		if f := v.FieldByName("Number"); f.IsValid() && f.Kind() == reflect.Uint16 {
			return uint16(f.Uint()) //nolint:gosec // read from a uint16
		}
	}
	return 0
}
