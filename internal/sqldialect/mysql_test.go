// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldialect_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/sqldialect"
)

type mysqlError struct{ Number uint16 }

func (*mysqlError) Error() string { return "MySQL server error" }

type wrongNumberError struct{ Number int }

func (wrongNumberError) Error() string { return "non-MySQL error" }

type stringError string

func (e stringError) Error() string { return string(e) }

func TestMySQLErrorNumber(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		err  error
		want uint16
	}{
		{name: "nil"},
		{name: "no number", err: errors.New("boom")},
		{name: "non-struct", err: stringError("boom")},
		{name: "wrong field type", err: wrongNumberError{Number: 1061}},
		{name: "typed nil", err: (*mysqlError)(nil)},
		{name: "duplicate index", err: &mysqlError{Number: 1061}, want: 1061},
		{name: "deadlock", err: &mysqlError{Number: 1213}, want: 1213},
		{name: "wrapped", err: fmt.Errorf("query: %w", &mysqlError{Number: 1061}), want: 1061},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, sqldialect.MySQLErrorNumber(tc.err))
		})
	}
}
