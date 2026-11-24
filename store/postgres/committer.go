// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/altessa-s/go-atlas-core/retry"

	corecontext "github.com/altessa-s/go-atlas-core/context"
	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

var (
	// ErrDifferentDatabase indicates a nested transaction selected another pool.
	ErrDifferentDatabase = errors.New("transaction belongs to a different database")
	// ErrUnsafeReplay indicates an attempt to replay a joined transaction or independently commit inside a replayable body.
	ErrUnsafeReplay = errors.New("independent transaction and whole-body replay cannot be combined")
	// ErrTransactionPanicked indicates a joined transaction panicked and its caller recovered.
	ErrTransactionPanicked = errors.New("joined transaction panicked")
)

type uncertainCommit struct{ error }

func (e *uncertainCommit) Unwrap() error    { return e.error }
func (e *uncertainCommit) GoString() string { return e.Error() }

// Committer runs units of work through [WithTransaction] on a caller-owned pool.
type Committer struct {
	db      *sql.DB
	timeout time.Duration
	policy  *retry.Policy
}

// NewCommitter constructs a committer without performing I/O. Whole-body replay is opt-in.
func NewCommitter(db *sql.DB, opts ...CommitterOption) (*Committer, error) {
	if db == nil {
		return nil, coreerrs.WrapOperation(coreerrs.ErrInvalidArgument, "nil database")
	}
	o := newCommitterOptions(opts...)
	if o.txRetries < 0 || o.txTimeout < 0 || o.txRetryDelay < 0 {
		return nil, coreerrs.WrapOperation(coreerrs.ErrInvalidArgument, "negative transaction retries, delay or timeout")
	}
	c := &Committer{db: db, timeout: o.txTimeout}
	if o.txRetries > 0 {
		if o.txRetryDelay == 0 {
			return nil, coreerrs.WrapOperation(coreerrs.ErrInvalidArgument, "replay requires a positive retry delay")
		}
		c.policy = retry.NewPolicy(
			retry.WithMaxAttempts(o.txRetries),
			retry.WithShouldRetry(func(err error) bool {
				kind, _ := Classify(err)
				return kind.RetryableTransaction() && !errors.Is(err, ErrTransactionPanicked)
			}),
			retry.WithNextDelay(retry.Exponential(o.txRetryDelay)),
			retry.WithJoinLastError(),
		)
	}
	return c, nil
}

// Transaction identifies the open write transaction of any pool in ctx. Use it as the
// uow.Detector so another pool's transaction is recognized as foreign even outside Run.
func Transaction(ctx context.Context) (any, bool) {
	t := attached(ctx)
	if t == nil || t.readOnly || !t.open() {
		return nil, false
	}
	return t.tx, true
}

// Detect identifies an open write transaction on this committer's selected pool.
func (c *Committer) Detect(ctx context.Context) (any, bool) {
	t := attached(ctx)
	if t == nil || t.db != selectedPool(ctx, c.db) {
		return nil, false
	}
	return Transaction(ctx)
}

// CommitOutcomeUnknown tells a uow.Runner whether the server may have committed the writes.
func (c *Committer) CommitOutcomeUnknown(err error) bool {
	_, ok := errors.AsType[*uncertainCommit](err)
	return ok
}

// WithTransaction runs fn once by default. Replay applies only to root transactions and requires a replay-safe callback.
func (c *Committer) WithTransaction(ctx context.Context, fn func(context.Context) error) error {
	if t := attached(ctx); t != nil {
		if !t.open() {
			return sql.ErrTxDone
		}
		if c.policy != nil {
			return ErrUnsafeReplay
		}
		if t.readOnly {
			return errors.New("unit of work requires a write transaction")
		}
		return WithTransaction(ctx, c.db, fn)
	}
	ctx, cancel := corecontext.WithMaxTimeout(ctx, c.timeout)
	defer cancel()
	if c.policy == nil {
		return WithTransaction(ctx, c.db, fn)
	}
	err := c.policy.Do(ctx, func(ctx context.Context) error {
		return WithTransaction(ctx, c.db, func(txCtx context.Context) error {
			attached(txCtx).life.replay = true
			return fn(txCtx)
		})
	})
	if err != nil && ctx.Err() != nil {
		if !errors.Is(err, ctx.Err()) {
			err = errors.Join(err, ctx.Err())
		}
		// The retry policy can join an application-provided cancellation cause.
		return diagnostic("transaction replay", err)
	}
	return err
}
