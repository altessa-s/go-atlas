// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package uow is an in-process unit of work that pairs a database transaction
// with non-transactional side effects via post-commit compensation (an
// in-process saga).
//
// # Why
//
// A synchronous-in-transaction handler (e.g. one driven by
// [github.com/altessa-s/go-atlas/messaging/eventbus]) can only roll back the
// transactional store: aborting the transaction undoes the database writes but
// not any non-transactional effect — an S3/blobstore put, a cache mutation, an
// external call. uow closes that gap without an outbox or distributed
// transaction.
//
// # Model
//
//  1. Transactional work runs inside the transaction through a [Committer].
//  2. Non-transactional effects do NOT run inside the transaction — they are
//     registered with [OnCommit] and applied exactly once AFTER a successful
//     commit.
//  3. If an effect fails after the commit, the effects already applied are
//     compensated in LIFO order.
//
// Applying after the commit means a driver-retried transaction never duplicates
// an effect, and an aborted transaction simply never runs them (a clean
// rollback with no compensation). Compensation covers the one remaining case —
// an effect that fails after the commit.
//
// # Context and cancellation
//
// [Effect.Apply] runs on the caller's context, so it honors cancellation — the
// caller has walked away. [Effect.Compensate] runs on a
// [context.WithoutCancel] copy, so an undo is never skipped because the caller's
// context was canceled. WithCompensationTimeout bounds the complete pass
// (default 30s); callbacks must honor this deadline. An effect with a nil Compensate is best-effort /
// irreversible and is skipped during compensation.
//
// # After-commit hooks
//
// [Runner.AfterCommit] registers a hook: a fire-and-forget callback such as a
// cache invalidation. Hooks cannot fail the transaction; a hook panic is
// recovered and logged. They run after the commit in registration order,
// interleaved with effects, on a [context.WithoutCancel] copy of the caller's
// context, so a cancellation after a successful commit never skips them, and
// they still run when an earlier effect failed. With
// WithRunHooksOutsideTransaction a hook registered outside any unit of work and
// transaction runs at once; otherwise [ErrNoUnitOfWork] is returned.
//
// # Retries, nesting and foreign transactions
//
// Every callback attempt of the [Committer] gets a fresh unit; the unit of an
// aborted attempt is closed, so a driver retry discards its registrations and
// a retained context of it returns [ErrScopeClosed]. A [Runner.Run] inside the
// body of the same Runner joins the enclosing unit: it calls the body directly,
// and its registrations apply only after the outermost commit. With
// WithDetector (store/mongo.Transaction has the [Detector] shape), each unit is
// bound to the transaction its attempt runs in, and Run, AfterCommit and
// [OnCommit] return [ErrForeignTransaction] when the context is inside another
// transaction: one the Runner did not start, or another client's transaction
// started inside the body, which may abort while the unit's commits. They also
// return it inside an open unit of another Runner.
//
// # Commit outcome
//
// When the Committer fails after the last body attempt returned nil, the
// commit may or may not have happened: Run returns an [*UnknownOutcomeError]
// (matching [ErrUnknownOutcome] and the Committer error), unless the committer reports
// a definitive abort through CommitOutcomeUnknown(error) bool. A body that returned
// an error, or that panicked and was recovered by the Committer (as
// store/mongo does, with ErrTransactionPanicked), is a known abort and its error
// is returned as is. WithDetector also checks that a successful root body
// leaves its captured transaction running. An ended or replaced transaction
// returns [ErrForeignTransaction] and discards registrations, without
// [ErrUnknownOutcome]. Without a detector, the Committer must report an explicit
// abort as an error. Bodies must leave transaction completion to the Committer.
//
// # Usage
//
//	runner := uow.New(mongo, logger,
//		uow.WithDetector(datamongo.Transaction),
//		uow.WithRunHooksOutsideTransaction(),
//	)
//	err := runner.Run(ctx, func(txCtx context.Context) error {
//		if err := repo.Save(txCtx, doc); err != nil {
//			return err
//		}
//		return runner.AfterCommit(txCtx, func(ctx context.Context) { cache.Invalidate(ctx, doc.ID) })
//	})
//
// # Durability caveat
//
// There is no outbox or durable relay: a crash between the commit and applying
// the effects leaves the database and the external store inconsistent. This is a
// deliberate trade-off for a single-process design. For durable, cross-service
// rollback use [github.com/altessa-s/go-atlas/reliability/saga] instead.
package uow
