// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package livebackend_test

import (
	"crypto/rand"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"net"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/tests/integration/internal/livebackend"
)

type failingDriver struct{ cause error }

func (d failingDriver) Open(string) (driver.Conn, error) { return nil, d.cause }

type report struct {
	testing.TB
	result, message string
}

func (r *report) Fatalf(format string, args ...any) {
	r.result = "fail"
	r.message = fmt.Sprintf(format, args...)
	panic("reported")
}
func (r *report) Skipf(format string, args ...any) {
	r.result = "skip"
	r.message = fmt.Sprintf(format, args...)
	panic("reported")
}

// Serial because the target reads its override from the process environment.
func TestSQLTargetPreflight(t *testing.T) {
	const secret = "driver-credential"
	dial := &net.OpError{Op: "dial", Err: fmt.Errorf("%s: %w", secret, syscall.ECONNREFUSED)}
	for _, tc := range []struct {
		name, override, want string
		err                  error
	}{
		{"local unavailable", "", "skip", dial}, {"configured unavailable", "explicit", "fail", dial}, {"local bad credentials", "", "fail", errors.New(secret)}, {"configured invalid", "explicit", "fail", errors.New(secret)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("LIVEBACKEND_TEST_DSN", tc.override)
			driverName := "preflight-" + rand.Text()
			sql.Register(driverName, failingDriver{cause: tc.err})
			target := livebackend.SQLTarget{Name: "postgres", Driver: driverName, Env: "LIVEBACKEND_TEST_DSN", DefaultDSN: "default"}
			r := &report{TB: t}
			require.PanicsWithValue(t, "reported", func() { target.Open(r) })
			require.Equal(t, tc.want, r.result)
			require.False(t, strings.Contains(r.message, secret), "driver text in test output")
		})
	}
}
