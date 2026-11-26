// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package uow

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/altessa-s/go-atlas-core/runtime/panics"
	"github.com/altessa-s/go-atlas/internal/txscope"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	slogx "github.com/altessa-s/go-slogx"
)

var (
	// ErrNoUnitOfWork is returned by [OnCommit] when ctx carries no unit of
	// work (OnCommit was called outside a [Runner.Run] body), and by
	// [Runner.AfterCommit] in the same situation unless
	// WithRunHooksOutsideTransaction is set.
	ErrNoUnitOfWork = errors.New("uow: no unit of work in context")

	// ErrForeignTransaction is returned by [Runner.Run], [Runner.AfterCommit]
	// and [OnCommit] when ctx carries a running transaction other than the one
	// the unit of work runs in (reported by the configured or built-in detector: a
	// transaction the Runner did not start, or another client's transaction
	// started inside it), or an open unit of work of another Runner. The body,
	// effect or hook is not called or registered. It also rejects a successful
	// root body whose transaction is no longer running or has changed identity;
	// that attempt's registrations are discarded without ErrUnknownOutcome.
	ErrForeignTransaction = errors.New("uow: context carries a transaction this runner did not start")

	// ErrScopeClosed is returned when ctx carries a unit of work that has
	// already ended: a context retained after [Runner.Run] returned, or one
	// of an attempt the driver aborted and retried.
	ErrScopeClosed = errors.New("uow: unit of work is closed")

	// ErrUnknownOutcome is matched (errors.Is) by an [*UnknownOutcomeError]:
	// the body succeeded but the commit did not report success, so the
	// writes may or may not have committed.
	ErrUnknownOutcome = errors.New("uow: transaction outcome unknown")
)

// Committer runs fn inside a transaction, passing a derived, transaction-scoped
// context to the body. A nil result from WithTransaction confirms a commit.
// Use [WithCommitConfirmation] to report a successful commit even when a
// later operation, such as a post-commit observer, fails. Implement [TransactionDetector] if WithTransaction can join an
// existing transaction.
// A committer may implement [OutcomeClassifier] to distinguish an
// ambiguous commit from a definitive abort, including failure to start a subsequent attempt.
type Committer interface {
	WithTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}

// WithCommitConfirmation lets a Committer report the outcome of the transaction
// whose context it passes to its body. Call the returned function only after
// that transaction commits. It is nil when the caller did not request a report.
func WithCommitConfirmation(ctx context.Context) (context.Context, func()) {
	return txscope.StartCommitReport(ctx)
}

// OutcomeClassifier optionally distinguishes an ambiguous commit error from a definitive abort.
type OutcomeClassifier interface {
	CommitOutcomeUnknown(error) bool
}

// OutcomeCommitter exposes commit success separately for callers of
// WithTransactionOutcome. [Runner] always calls [Committer.WithTransaction]
// so an embedding wrapper's override still participates in the transaction.
type OutcomeCommitter interface {
	WithTransactionOutcome(ctx context.Context, fn func(ctx context.Context) error) (committed bool, err error)
}

// TransactionDetector supplies a default detector for a [Committer] when the
// caller has not supplied WithDetector.
type TransactionDetector interface {
	DetectTransaction(ctx context.Context) (tx any, ok bool)
}

// Detector reports the innermost running database transaction ctx carries,
// of any client: ok is false when there is none, and tx identifies it —
// equal for every context of one transaction (a joined one included) and
// different for another transaction (another client's started inside it,
// for example). tx must be comparable with ==. store/mongo.Transaction
// satisfies it.
type Detector func(ctx context.Context) (tx any, ok bool)

// UnknownOutcomeError reports a root transaction whose body returned nil but
// whose commit did not report success: the data may or may not have
// committed. Err is the original [Committer] error; driver labels stay
// reachable through errors.As. A body that returned an error, or that
// panicked and was recovered by the Committer, never produces it.
type UnknownOutcomeError struct {
	Err error
}

// Error implements [error].
func (e *UnknownOutcomeError) Error() string {
	if e == nil || e.Err == nil {
		return ErrUnknownOutcome.Error()
	}
	return ErrUnknownOutcome.Error() + ": " + e.Err.Error()
}

// Unwrap returns [ErrUnknownOutcome] and the underlying Committer error so
// both are matched by errors.Is and errors.As.
func (e *UnknownOutcomeError) Unwrap() []error {
	if e == nil {
		return nil
	}
	if e.Err == nil {
		return []error{ErrUnknownOutcome}
	}
	return []error{ErrUnknownOutcome, e.Err}
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

// entry is one registration of a unit: an [Effect] (OnCommit) or a hook
// (AfterCommit).
type entry struct {
	scope  *txscope.Savepoint
	effect Effect
	hook   func(ctx context.Context)
}

// unit holds the registrations of one body attempt of a root Run. It is safe
// for concurrent registration; once closed it accepts nothing.
type unit struct {
	commit  *txscope.CommitReport // confirmation from the adapter that ran this body
	scope   *txscope.Savepoint    // savepoint at the start of this body attempt
	runner  *Runner
	tx      any // the transaction the attempt runs in, as the detector identifies it; nil without a detector
	mu      sync.Mutex
	closed  bool
	entries []entry
}

func (u *unit) add(e entry) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.closed {
		return ErrScopeClosed
	}
	u.entries = append(u.entries, e)
	return nil
}

func (u *unit) isClosed() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.closed
}

// close closes u and returns its registrations; later calls return nil.
func (u *unit) close() []entry {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.closed = true
	entries := u.entries
	u.entries = nil
	return entries
}

// unitKey keys the unit of one Runner, so a Runner finds its own unit even
// below another Runner's.
type unitKey struct {
	txscope.Key
	r *Runner
}

// activeKey keys the innermost unit, used by [OnCommit] and by foreign
// detection.
type activeKey struct{ txscope.Key }

func withUnit(ctx context.Context, u *unit) context.Context {
	ctx = context.WithValue(ctx, unitKey{r: u.runner}, u)
	return context.WithValue(ctx, activeKey{}, u)
}

// Runner executes a transactional body and applies its registered post-commit
// effects and hooks, compensating effects on failure. It holds no
// package-level state and is safe for concurrent use.
type Runner struct {
	committer           Committer
	logger              *slog.Logger
	compensationTimeout time.Duration
	detector            Detector
	runHooksOutside     bool
}

// New returns a Runner that commits through committer. A nil logger falls back
// to [slog.Default]; the returned Runner tags its logs with the uow module.
func New(committer Committer, logger *slog.Logger, opts ...Option) *Runner {
	if logger == nil {
		logger = slog.Default()
	}
	o := newOptions(opts...)
	if o.detector == nil {
		if detector, ok := committer.(TransactionDetector); ok {
			o.detector = detector.DetectTransaction
		}
	}
	return &Runner{
		committer:           committer,
		logger:              logger.With(slogx.Module("pkg:uow")),
		compensationTimeout: o.compensationTimeout,
		detector:            o.detector,
		runHooksOutside:     o.runHooksOutsideTransaction,
	}
}

// Run executes body in a unit of work of r.
//
// Root (ctx carries no unit of r): Run calls the Committer, which starts a
// transaction. Every body attempt gets a fresh unit and the unit of an
// aborted attempt is closed, so a driver retry discards its registrations.
// When the Committer confirms the transaction committed, Run applies
// the registrations of the last attempt in registration order (see
// [OnCommit] and [Runner.AfterCommit]) and returns any post-commit and effect errors.
// When the transaction fails nothing is applied and the error is classified:
// if the last attempt returned an error, or never returned (it panicked and
// the Committer recovered the panic), the Committer error is returned as is;
// if the last attempt returned nil, the failure happened at or after commit
// and Run returns an [*UnknownOutcomeError] unless the committer reports a definitive abort. With a detector, Run first
// returns [ErrForeignTransaction] when ctx is inside a transaction r did not
// start. After a successful root body, the detector must still report the
// captured transaction as running; otherwise ErrForeignTransaction is returned
// and registrations are discarded without UnknownOutcomeError. Without a
// detector, the Committer must report an explicit abort as an error.
//
// Nested (ctx carries an open unit of r): Run calls body directly and returns
// its error as is. Registrations join the enclosing unit and apply only after
// the outermost commit; the root decides the abort. A nested Run never
// commits. A panic closes the unit and propagates. If recovered inside the root body,
// the root refuses commit with [ErrScopeClosed].
//
// Closed (ctx carries a closed unit of r): [ErrScopeClosed]; body is not called.
func (r *Runner) Run(ctx context.Context, body func(ctx context.Context) error) error {
	if u, ok := ctx.Value(unitKey{r: r}).(*unit); ok && u != nil {
		if err := r.joinable(ctx, u); err != nil {
			return err
		}
		if active, _ := ctx.Value(activeKey{}).(*unit); active != u {
			ctx = context.WithValue(ctx, activeKey{}, u)
		}
		// Closing during unwind also rejects a root whose caller recovers this panic.
		defer func() {
			if r := recover(); r != nil {
				u.close()
				panic(r)
			}
		}()
		return body(ctx)
	}
	if r.inForeignTransaction(ctx) {
		return ErrForeignTransaction
	}
	return r.runRoot(ctx, body)
}

func (r *Runner) runRoot(ctx context.Context, body func(ctx context.Context) error) error {
	var (
		u        *unit
		returned bool
		bodyErr  error
	)
	defer func() {
		if u != nil {
			u.close() // also on a panic the Committer propagates
		}
	}()
	runBody := func(txCtx context.Context) error {
		if u != nil {
			u.close() // the previous attempt was aborted
		}
		u = &unit{runner: r, scope: txscope.Current(txCtx), commit: txscope.ReportFrom(txCtx)}
		if r.detector != nil {
			u.tx, _ = r.detector(txCtx)
		}
		returned = false
		bodyErr = body(withUnit(txCtx, u))
		if bodyErr == nil {
			// A driver may return success after the body explicitly aborts.
			// Validate before it commits, while this attempt must still be active.
			bodyErr = r.joinable(txCtx, u)
		}
		returned = true
		return bodyErr
	}
	err := r.committer.WithTransaction(txscope.WithCommitReporting(ctx), runBody)
	if u == nil {
		return err
	}
	entries := u.close()
	switch {
	case err == nil || (returned && bodyErr == nil && u.commit.Confirmed()):
		effectErr := r.apply(ctx, entries, u.scope)
		if err == nil {
			return effectErr
		}
		return errors.Join(err, effectErr)
	case returned && bodyErr == nil:
		// Some backends can distinguish a definitive rollback or a failed retry BEGIN
		// from loss of the reply to COMMIT.
		if reporter, ok := r.committer.(OutcomeClassifier); ok && !reporter.CommitOutcomeUnknown(err) {
			return err
		}
		return &UnknownOutcomeError{Err: err}
	default:
		return err
	}
}

// joinable checks that ctx, carrying the unit u of r, may join it: u is open and, with a detector, ctx is inside the very
// transaction u's attempt runs in — not merely inside some transaction, such as another client's started within it, whose abort
// would leave u's registrations applied for writes that never committed.
func (r *Runner) joinable(ctx context.Context, u *unit) error {
	if u.isClosed() || !txscope.Current(ctx).OpenWithin(u.scope) {
		return ErrScopeClosed
	}
	if r.detector != nil {
		if tx, ok := r.detector(ctx); !ok || tx != u.tx {
			return ErrForeignTransaction
		}
	}
	return nil
}

// inForeignTransaction reports, for a ctx without a unit of r, whether it is
// inside a detected transaction or a unit of work r did not start.
func (r *Runner) inForeignTransaction(ctx context.Context) bool {
	if r.detector != nil {
		if _, ok := r.detector(ctx); ok {
			return true
		}
	}
	other, ok := ctx.Value(activeKey{}).(*unit)
	return ok && other != nil && other.runner != r && !other.isClosed()
}

// AfterCommit registers hook on the open unit of r in ctx. Hooks cannot fail
// the transaction: a hook logs its own errors, and a hook panic is recovered
// and logged while the remaining registrations still apply. After the root
// commit, hooks run in registration order (interleaved with effects) on a
// [context.WithoutCancel] copy of the root caller's context, so a cancellation
// after a successful commit never skips them; they run even when an earlier
// effect failed.
//
// Without a unit of r in ctx: [ErrForeignTransaction] inside a foreign
// transaction (as reported by a detector); otherwise hook runs at once on ctx when
// WithRunHooksOutsideTransaction is set, or [ErrNoUnitOfWork] is returned. A
// closed unit returns [ErrScopeClosed]. A nil hook is accepted and ignored.
func (r *Runner) AfterCommit(ctx context.Context, hook func(ctx context.Context)) error {
	if u, ok := ctx.Value(unitKey{r: r}).(*unit); ok && u != nil {
		if err := r.joinable(ctx, u); err != nil {
			return err
		}
		if hook == nil {
			return nil
		}
		return u.add(entry{scope: txscope.Current(ctx), hook: hook})
	}
	if r.inForeignTransaction(ctx) {
		return ErrForeignTransaction
	}
	if !r.runHooksOutside {
		return ErrNoUnitOfWork
	}
	r.runHook(ctx, hook)
	return nil
}

// apply runs the registrations in order. An effect's Apply runs on ctx; on the
// first failure the applied effects are compensated (LIFO) and later effects
// are skipped. Hooks run on a cancellation-detached ctx regardless.
func (r *Runner) apply(ctx context.Context, entries []entry, rootScope *txscope.Savepoint) error {
	hookCtx := context.WithoutCancel(ctx)
	var (
		applied []Effect
		failed  error
	)
	for _, e := range entries {
		if !e.scope.ReleasedWithin(rootScope) {
			continue
		}
		if e.hook != nil {
			r.runHook(hookCtx, e.hook)
			continue
		}
		if failed != nil || e.effect.Apply == nil {
			continue
		}
		if err := runCallback(ctx, e.effect.Apply); err != nil {
			applyErr := coreerrs.WrapOperationWithContext(err, "apply post-commit effect", e.effect.Label)
			failed = errors.Join(applyErr, r.compensate(ctx, applied))
			continue
		}
		applied = append(applied, e.effect)
	}
	return failed
}

// compensate undoes applied effects in LIFO order on a non-cancelable context,
// skipping nil Compensate funcs and logging (an orphaned external state is
// serious) and aggregating any compensation failures.
func (r *Runner) compensate(ctx context.Context, applied []Effect) error {
	compCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), r.compensationTimeout)
	defer cancel()
	var errs error
	for _, e := range slices.Backward(applied) {
		if err := compCtx.Err(); err != nil {
			return errors.Join(errs, err)
		}
		if e.Compensate == nil {
			continue
		}
		if err := runCallback(compCtx, e.Compensate); err != nil {
			r.logger.ErrorContext(compCtx, "post-commit effect compensation failed",
				slog.String("effect", e.Label), slog.Any("error", err))
			errs = errors.Join(errs, coreerrs.WrapOperationWithContext(err, "compensate post-commit effect", e.Label))
		}
	}
	return errs
}

// runHook calls hook on ctx, recovering and logging a panic.
func (r *Runner) runHook(ctx context.Context, hook func(ctx context.Context)) {
	if hook == nil {
		return
	}
	if err := runCallback(ctx, func(ctx context.Context) error {
		hook(ctx)
		return nil
	}); err != nil {
		r.logger.ErrorContext(ctx, "after-commit hook panicked", slog.Any("error", err))
	}
}

// runCallback converts a panic to an error without including its payload.
func runCallback(ctx context.Context, fn func(context.Context) error) (err error) {
	defer panics.HandleWithOpts(ctx, panics.NewHandleOpts().SetReallyPanic(false), func(context.Context, any) {
		err = errors.New("post-commit callback panicked")
	})
	return fn(ctx)
}

// OnCommit registers a post-commit effect from within a [Runner.Run] body (for
// example a handler) on the innermost unit of work in ctx. It returns
// [ErrNoUnitOfWork] if ctx carries no unit of work, [ErrScopeClosed] if that
// unit has already ended, and, when its Runner has a detector,
// [ErrForeignTransaction] if ctx is inside another transaction than the
// unit's (see [Runner.AfterCommit]).
func OnCommit(ctx context.Context, e Effect) error {
	u, ok := ctx.Value(activeKey{}).(*unit)
	if !ok || u == nil {
		return ErrNoUnitOfWork
	}
	if err := u.runner.joinable(ctx, u); err != nil {
		return err
	}
	return u.add(entry{scope: txscope.Current(ctx), effect: e})
}
