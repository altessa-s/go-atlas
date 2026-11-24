// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-core/types/redacted"
	"github.com/altessa-s/go-atlas/internal/sqldialect"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

type failingConnector struct {
	driver.Connector
	cause error
}

func (c failingConnector) Connect(context.Context) (driver.Conn, error) { return nil, c.cause }

func TestConnectSafeRenderingPreservesDriverContracts(t *testing.T) {
	t.Parallel()
	diagnostic := &pgconn.PgError{Message: "credential-in-driver-text", Code: "23505", ConstraintName: "unique_name"}
	for _, tc := range []struct {
		name  string
		cause error
	}{
		{"context canceled", context.Canceled}, {"deadline", context.DeadlineExceeded}, {"connection closed", sql.ErrConnDone}, {"transaction closed", sql.ErrTxDone}, {"bad connection", driver.ErrBadConn}, {"pool defaults", ErrPooledTimeoutDefaults},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cause := errors.Join(diagnostic, tc.cause)
			c := connector{Connector: failingConnector{cause: cause}}
			_, err := c.Connect(t.Context())
			require.Error(t, err)
			require.False(t, strings.Contains(fmt.Sprintf("%+v", err), diagnostic.Message), "driver text escaped")
			require.ErrorIs(t, err, tc.cause)
			pgErr, ok := errors.AsType[*pgconn.PgError](err)
			require.True(t, ok)
			require.Same(t, diagnostic, pgErr)
		})
	}
}

type stateOnly string

func (s stateOnly) Error() string    { return "driver error" }
func (s stateOnly) SQLState() string { return string(s) }

func TestClassifyDriverNeutralState(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		state string
		kind  ErrorKind
	}{{"23505", Unique}, {"40P01", Deadlock}, {"08001", ConnectionLost}, {"08bad", Unknown}, {"08credential", Unknown}} {
		t.Run(tc.state, func(t *testing.T) {
			t.Parallel()
			kind, name := Classify(sqldialect.SafeError(nil, "connect", stateOnly(tc.state)))
			require.Equal(t, tc.kind, kind)
			require.Empty(t, name)
		})
	}
}

func TestOpenHidesDSNPasswords(t *testing.T) {
	t.Parallel()
	for _, tc := range testhelpers.PostgresSecretDSNs() {
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			_, err := Open(t.Context(), redacted.RedactedString(tc.DSN))
			require.Error(t, err)
			for _, secret := range tc.Secrets {
				require.False(t, strings.Contains(err.Error(), secret), "password in Open error")
			}
		})
	}
}
