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
// [github.com/altessa-s/go-atlas/domain/eventbus]) can only roll back the
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
// context was canceled. An effect with a nil Compensate is best-effort /
// irreversible and is skipped during compensation.
//
// # Durability caveat
//
// There is no outbox or durable relay: a crash between the commit and applying
// the effects leaves the database and the external store inconsistent. This is a
// deliberate trade-off for a single-process design. For durable, cross-service
// rollback use [github.com/altessa-s/go-atlas/data/saga] instead.
package uow
