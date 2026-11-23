// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb

import (
	"context"
	"database/sql"
	"errors"

	"github.com/altessa-s/go-atlas/internal/sqldialect"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

// ErrRestartTransaction indicates that a caller-owned WithTx transaction must
// be rolled back and retried in full. The underlying driver error is preserved.
var ErrRestartTransaction = errors.New("sqldb: restart the business transaction")

type sqlStateError interface {
	error
	SQLState() string
}

func (s *options) resolveTx(ctx context.Context) (*sql.Tx, bool) {
	if tx, ok := ctx.Value(txKey{}).(*sql.Tx); ok && tx != nil {
		return tx, true
	}
	if s.txResolver != nil {
		return s.txResolver(ctx)
	}
	return nil, false
}

func (s *Storage) operationError(ctx context.Context, err error, op string) error {
	if _, ok := s.db.Tx(ctx); ok && restartRequired(err) {
		err = errors.Join(ErrRestartTransaction, err)
	}
	return coreerrs.WrapOperation(err, op)
}

// restartRequired follows outbox's driver-independent error shapes. MariaDB
// 1020 is a stale snapshot under innodb_snapshot_isolation; 1213 is a deadlock.
func restartRequired(err error) bool {
	if e, ok := errors.AsType[sqlStateError](err); ok {
		if state := e.SQLState(); state == "40001" || state == "40P01" {
			return true
		}
	}
	n := sqldialect.MySQLErrorNumber(err)
	return n == 1020 || n == 1213
}
