// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package uow

import (
	"context"
	"errors"
	"log/slog"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	slogx "github.com/altessa-s/go-atlas/observability/slog"
)

// ErrNoUnitOfWork is returned by [OnCommit] when ctx carries no active unit of
// work (OnCommit was called outside a [Runner.Run] body). Matchable with
// errors.Is.
var ErrNoUnitOfWork = errors.New("uow: no unit of work in context")

// Committer runs fn inside a transaction, passing a transaction-scoped context
// that propagates the unit of work to the body. It is satisfied by
// *data/mongo.Mongo (its WithTransaction); future backends supply their own. The
// passed context must derive from the one given so OnCommit can find the unit.
type Committer interface {
	WithTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}

// Effect is a non-transactional side effect deferred until after the commit.
type Effect struct {
	// Label is used for error and log context.
	Label string
	// Apply runs the effect exactly once, post-commit, on the caller's context.
	Apply func(ctx context.Context) error
	// Compensate undoes a successful Apply during rollback; a nil Compensate
	// marks the effect best-effort / irreversible and is skipped.
	Compensate func(ctx context.Context) error
}

// unit holds the effects registered during one Run body. It is not safe for
// concurrent use: dispatch is synchronous in the body's goroutine.
type unit struct {
	effects []Effect
}

// ctxKey is the private context key under which the active unit is carried.
type ctxKey struct{}

// Runner executes a transactional body and applies its registered post-commit
// effects, compensating on failure.
type Runner struct {
	committer Committer
	logger    *slog.Logger
}

// New returns a Runner that commits through committer. A nil logger falls back
// to [slog.Default]; the returned Runner tags its logs with the uow module.
func New(committer Committer, logger *slog.Logger) *Runner {
	if logger == nil {
		logger = slog.Default()
	}
	return &Runner{committer: committer, logger: logger.With(slogx.Module("pkg:uow"))}
}

// Run executes body inside a transaction. On a successful commit it applies the
// effects registered via [OnCommit] in registration order; if a later effect
// fails it compensates the already-applied ones in LIFO order and returns the
// apply error joined with any compensation error. If the transaction aborts (the
// body or the commit fails) no effect is applied and that error is returned.
func (r *Runner) Run(ctx context.Context, body func(ctx context.Context) error) error {
	u := &unit{}
	txCtx := context.WithValue(ctx, ctxKey{}, u)

	if err := r.committer.WithTransaction(txCtx, func(innerCtx context.Context) error {
		// Reset on every attempt: the driver may retry the callback, and only
		// the effects of the committed attempt must survive.
		u.effects = u.effects[:0]
		return body(innerCtx)
	}); err != nil {
		return err
	}

	return r.apply(ctx, u.effects)
}

// apply runs each effect's Apply in order on ctx; on the first failure it
// compensates the already-applied effects (LIFO) and returns the joined error.
func (r *Runner) apply(ctx context.Context, effects []Effect) error {
	applied := make([]Effect, 0, len(effects))
	for _, e := range effects {
		if e.Apply == nil {
			continue
		}
		if err := e.Apply(ctx); err != nil {
			applyErr := coreerrs.WrapOperationWithContext(err, "apply post-commit effect", e.Label)
			return errors.Join(applyErr, r.compensate(ctx, applied))
		}
		applied = append(applied, e)
	}
	return nil
}

// compensate undoes applied effects in LIFO order on a non-cancelable context,
// skipping nil Compensate funcs and logging (an orphaned external state is
// serious) and aggregating any compensation failures.
func (r *Runner) compensate(ctx context.Context, applied []Effect) error {
	compCtx := context.WithoutCancel(ctx)
	var errs error
	for i := len(applied) - 1; i >= 0; i-- {
		e := applied[i]
		if e.Compensate == nil {
			continue
		}
		if err := e.Compensate(compCtx); err != nil {
			r.logger.ErrorContext(compCtx, "post-commit effect compensation failed",
				slog.String("effect", e.Label), slog.Any("error", err))
			errs = errors.Join(errs, coreerrs.WrapOperationWithContext(err, "compensate post-commit effect", e.Label))
		}
	}
	return errs
}

// OnCommit registers a post-commit effect from within a [Runner.Run] body (for
// example a handler). It returns [ErrNoUnitOfWork] if ctx carries no active unit
// of work.
func OnCommit(ctx context.Context, e Effect) error {
	u, ok := ctx.Value(ctxKey{}).(*unit)
	if !ok || u == nil {
		return ErrNoUnitOfWork
	}
	u.effects = append(u.effects, e)
	return nil
}
