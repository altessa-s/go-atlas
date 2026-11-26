// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"
	"errors"
	"log/slog"
	"runtime/debug"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas-core/runtime/panics"

	corecontext "github.com/altessa-s/go-atlas-core/context"
	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// WithTransaction executes a function within a MongoDB transaction using MongoDB's native
// retry logic. It uses the driver's built-in Session.WithTransaction method which automatically
// handles transient errors, commit retries, and transaction lifecycle management.
//
// The transaction uses configurable options from TransactionOptions including:
//   - Read concern, write concern, and read preference
//   - MongoDB's optimized retry logic for transient errors
//   - Automatic transaction lifecycle management
//
// Parameters:
//   - ctx: Context for controlling transaction timeout and cancellation
//   - handler: Function to execute within the transaction. It receives a session context
//     that must be used for all database operations within the transaction.
//
// Returns:
//   - error: Error if transaction fails, or ErrTransactionPanicked if the
//     handler panicked — the transaction is then aborted, never committed
//
// Joining: when ctx already carries a running transaction of this client
// ([InTransaction]), for example a store method called from another
// WithTransaction handler, the handler runs directly in that transaction
// instead of starting a second, independent one on a new session: its
// writes commit or abort with the enclosing transaction, its error is
// returned as is for the enclosing handler to propagate, and a panic is
// left to the enclosing transaction's owner. No timeout, metric or log of
// its own is applied. A transaction of another client is never joined.
//
// Example:
//
//	err := mongo.WithTransaction(ctx, func(sessCtx context.Context) error {
//		// Use sessCtx for all operations within the transaction
//		_, err := collection.InsertOne(sessCtx, document)
//		return err
//	})
func (m *Mongo) WithTransaction(ctx context.Context, handler func(ctx context.Context) error) error {
	return m.conn.withTransaction(ctx, m.Client(), handler)
}

func (m *connection) withTransaction(ctx context.Context, client *mongo.Client, handler func(context.Context) error) error {
	if joinableTransaction(ctx, client) {
		return handler(ctx)
	}

	stop := m.metrics.transactionDuration.Start()
	defer stop()

	// Apply transaction timeout (cap to DefaultTxTimeout)
	ctxWithTimeout, cancel := corecontext.WithMaxTimeout(ctx, DefaultTxTimeout)
	defer cancel()

	startTime := time.Now()

	sess, err := client.StartSession()
	if err != nil {
		return coreerrs.WrapOperation(err, "start session")
	}
	defer sess.EndSession(ctxWithTimeout)

	txnOpts := mongoOptions.Transaction()

	// Apply configurable transaction options
	if m.transactionOptions.ReadConcern != nil {
		txnOpts.SetReadConcern(m.transactionOptions.ReadConcern)
	}
	if m.transactionOptions.WriteConcern != nil {
		txnOpts.SetWriteConcern(m.transactionOptions.WriteConcern)
	}
	if m.transactionOptions.ReadPreference != nil {
		txnOpts.SetReadPreference(m.transactionOptions.ReadPreference)
	}

	var panicErr, callbackErr error
	_, err = sess.WithTransaction(ctxWithTimeout, func(sessCtx context.Context) (_ any, txErr error) {
		// Forward a safe sentinel to the global hooks while preserving the
		// stack and making the driver's transaction callback fail.
		defer panics.HandleWithOpts(sessCtx, panics.NewHandleOpts().SetReallyPanic(false), func(ctx context.Context, _ any) {
			panicErr = ErrTransactionPanicked
			txErr = panicErr
			if m.logger != nil {
				m.logger.ErrorContext(ctx, "transaction panicked", "stacktrace", string(debug.Stack()))
			}
		})
		defer func() {
			if recover() != nil {
				panic(ErrTransactionPanicked)
			}
		}()

		callbackErr = handler(sessCtx)
		if callbackErr != nil {
			return nil, callbackErr
		}
		return struct{}{}, nil
	}, txnOpts)

	// Panic errors take priority over regular errors
	if panicErr != nil {
		err = panicErr
	}

	if err != nil {
		m.metrics.transactionErrors.Inc()

		if m.logger != nil {
			m.logger.LogAttrs(ctx, slog.LevelError, "transaction failed",
				transactionErrorClass(err, callbackErr), slog.Int64("duration_ms", time.Since(startTime).Milliseconds()))
		}
		return err
	}

	if m.logger != nil {
		m.logger.Debug("transaction completed successfully",
			"duration_ms", time.Since(startTime).Milliseconds())
	}

	return nil
}

// transactionErrorClass reports only fixed classifications or numeric database codes.
func transactionErrorClass(err, callbackErr error) slog.Attr {
	switch {
	case errors.Is(err, ErrTransactionPanicked):
		return slog.String("error_class", "transaction_panicked")
	case errors.Is(err, context.Canceled):
		return slog.String("error_class", "context_canceled")
	case errors.Is(err, context.DeadlineExceeded):
		return slog.String("error_class", "deadline_exceeded")
	}
	if command, ok := errors.AsType[mongo.CommandError](err); ok {
		return slog.Group("error_class", slog.String("kind", "mongo_command"), slog.Int("code", int(command.Code)))
	}
	if _, ok := errors.AsType[mongo.ServerError](err); ok {
		return slog.String("error_class", "mongo_server")
	}
	if callbackErr != nil && errors.Is(err, callbackErr) {
		return slog.String("error_class", "callback_error")
	}
	return slog.String("error_class", "transaction_error")
}
