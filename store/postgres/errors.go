// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package postgres

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"io"
	"net"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/altessa-s/go-atlas/internal/sqldialect"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

// ErrPooledTimeoutDefaults indicates that an autocommit statement could run without a server-side timeout.
var ErrPooledTimeoutDefaults = errors.New("database role needs nonzero timeout defaults for pooled connections")

// ErrDatabaseMismatch indicates that pools or a transaction and pool belong to different databases.
var ErrDatabaseMismatch = ErrDifferentDatabase

var errInvalidDSN = errors.New("invalid PostgreSQL DSN")

// driverDiagnostic keeps the public error chain available for programmatic inspection while
// formatting only the operation, error type and validated SQLSTATE.
type driverDiagnostic struct {
	error
	cause error
}

func (e *driverDiagnostic) Unwrap() error    { return e.cause }
func (e *driverDiagnostic) GoString() string { return e.Error() }

func diagnostic(op string, err error) error {
	if err == nil {
		return nil
	}
	return &driverDiagnostic{error: sqldialect.SafeError(nil, op, err), cause: err}
}

// Diagnostic wraps the driver error of a storage statement for return across a storage boundary. Its text is
// "<op> failed (<Go type of err>)", with ", SQLSTATE <code>" when err carries a well-formed one: driver text, which may quote
// statement values or the connection string, is never formatted. The cause stays available to errors.Is, errors.AsType and
// [Classify]. A nil err returns nil.
func Diagnostic(op string, err error) error { return diagnostic(op, err) }

// ErrorKind identifies PostgreSQL diagnostics useful at a storage boundary.
type ErrorKind string

const (
	// Unknown indicates an unrecognized error or nil.
	Unknown ErrorKind = ""
	// Unique identifies a unique constraint violation (23505).
	Unique ErrorKind = "unique"
	// ForeignKey identifies a foreign key violation (23503).
	ForeignKey ErrorKind = "foreign_key"
	// Check identifies a check constraint violation (23514).
	Check ErrorKind = "check"
	// Serialization indicates the whole transaction can be retried (40001).
	Serialization ErrorKind = "serialization"
	// Deadlock indicates the whole transaction can be retried (40P01).
	Deadlock ErrorKind = "deadlock"
	// ConnectionBeforeSend means the driver guarantees the statement was never sent.
	ConnectionBeforeSend ErrorKind = "connection_before_send"
	// ConnectionLost means the server may have executed the statement.
	ConnectionLost ErrorKind = "connection_lost"
	// StatementTimeout identifies a server statement_timeout diagnostic.
	StatementTimeout ErrorKind = "statement_timeout"
	// Timeout identifies a client context deadline expiry.
	Timeout ErrorKind = "timeout"
	// Canceled identifies request cancellation, deadline expiry or a server cancel request.
	Canceled ErrorKind = "canceled"
)

// RetryableTransaction reports whether PostgreSQL aborted a transaction for a serialization failure or deadlock.
func (k ErrorKind) RetryableTransaction() bool { return k == Serialization || k == Deadlock }

// Classify returns the error kind and, for integrity violations, the constraint name.
// PostgreSQL uses SQLSTATE 57014 for both a statement timeout and user cancellation.
// The timeout distinction without a caller context requires English server messages (lc_messages=C);
// prefer [ClassifyContext] when the operation context is available.
func Classify(err error) (ErrorKind, string) {
	if err == nil {
		return Unknown, ""
	}
	if coreerrs.IsContextCanceledOrDeadlineExceeded(err) {
		return Canceled, ""
	}
	return classifyCommon(err, func(e *pgconn.PgError) ErrorKind {
		if strings.Contains(e.Message, "statement timeout") {
			return StatementTimeout
		}
		if strings.Contains(e.Message, "user request") {
			return Canceled
		}
		return Unknown
	})
}

// ClassifyContext classifies err using ctx to disambiguate SQLSTATE 57014
// independently of PostgreSQL's server message locale: a context deadline
// yields [Timeout], cancellation yields [Canceled], and an active context
// without a user-cancel message yields [StatementTimeout].
func ClassifyContext(ctx context.Context, err error) (ErrorKind, string) {
	if err == nil {
		return Unknown, ""
	}
	if coreerrs.IsContextDeadlineExceeded(err) {
		return Timeout, ""
	}
	if coreerrs.IsContextCanceled(err) {
		return Canceled, ""
	}
	var ctxErr error
	if ctx != nil {
		ctxErr = ctx.Err()
	}
	return classifyCommon(err, func(e *pgconn.PgError) ErrorKind {
		switch {
		case errors.Is(ctxErr, context.DeadlineExceeded):
			return Timeout
		case errors.Is(ctxErr, context.Canceled):
			return Canceled
		case strings.Contains(e.Message, "user request"):
			return Canceled
		default:
			return StatementTimeout
		}
	})
}

func classifyCommon(err error, classify57014 func(*pgconn.PgError) ErrorKind) (ErrorKind, string) {
	if _, ok := errors.AsType[*pgconn.ConnectError](err); ok {
		return ConnectionBeforeSend, ""
	}
	e, ok := errors.AsType[*pgconn.PgError](err)
	if !ok {
		e = &pgconn.PgError{Code: sqldialect.SQLState(err)}
	}

	switch e.Code {
	case "23505":
		return Unique, e.ConstraintName
	case "23503":
		return ForeignKey, e.ConstraintName
	case "23514":
		return Check, e.ConstraintName
	case "40001":
		return Serialization, ""
	case "40P01":
		return Deadlock, ""
	case "57014":
		return classify57014(e), ""
	case "57P01", "57P02", "57P03":
		return ConnectionLost, ""
	}
	severity := cmp.Or(e.SeverityUnlocalized, e.Severity)
	if strings.HasPrefix(e.Code, "08") || severity == "FATAL" || severity == "PANIC" {
		return ConnectionLost, ""
	}
	if sqldialect.SafeToRetry(err) {
		return ConnectionBeforeSend, ""
	}

	_, network := errors.AsType[net.Error](err)
	if network || errors.Is(err, net.ErrClosed) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, pgconn.ErrConnClosed) {
		return ConnectionLost, ""
	}
	return Unknown, ""
}

// UniqueViolation returns the violated unique constraint or index name.
func UniqueViolation(err error) (string, bool) {
	kind, name := Classify(err)
	if kind == Unique {
		return name, true
	}
	return "", false
}

// ResourceError maps absent and duplicate resources to application-supplied errors.
// A duplicate preserves the server diagnostic for constraint inspection.
func ResourceError(err, missing, duplicate error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return missing
	}
	if _, ok := UniqueViolation(err); ok {
		return errors.Join(duplicate, err)
	}
	return err
}
