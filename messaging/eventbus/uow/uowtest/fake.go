// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package uowtest

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/altessa-s/go-atlas-core/runtime/panics"
	"github.com/altessa-s/go-atlas/messaging/eventbus/uow"
)

// ErrPanicked is returned (wrapped) by [Fake.WithTransaction] for a callback panic when [Fake.RecoverPanics] is set, mirroring
// store/mongo.ErrTransactionPanicked.
var ErrPanicked = errors.New("uowtest: transaction callback panicked")

// Snapshotter is satisfied structurally by in-memory stores written inside a callback; they do not import this package.
type Snapshotter interface {
	// Snapshot captures the current state and returns a function that restores it.
	Snapshot() (restore func())
}

var (
	_ uow.Committer = (*Fake)(nil)
	_ uow.Detector  = Transaction
)

// Fake is a [uow.Committer] for unit tests; [Transaction] is its [uow.Detector]. Its zero value runs one attempt, no
// commit retry, no failure. A call inside a running transaction of the same Fake joins it, as store/mongo does (the innermost one: inside
// another Fake's attempt it starts its own, and blocks if that attempt runs inside one of its own). WithTransaction holds
// an internal mutex for its whole duration: root transactions on one Fake never overlap.
type Fake struct {
	// Attempts is the number of callback attempts per transaction (at least 1); every attempt but the last ends in a transient
	// abort and is rolled back.
	Attempts int
	// CommitRetries is the number of commit-only retries after the last attempt; the callback is not re-run.
	CommitRetries int
	// Fail, if set, is returned instead of committing; the last attempt is rolled back.
	Fail error
	// Unknown, if set, is called with the 1-based number of the transaction after its last attempt committed. A non-nil result is
	// returned instead of nil while the writes stay committed: the commit happened, its reply was lost.
	Unknown func(txn int) error
	// Abort, if set, is called with the transaction number after its last attempt; a non-nil result is returned instead of
	// committing and the attempt is rolled back (Fail for selected transactions only).
	Abort func(txn int) error
	// RecoverPanics, if set, recovers a callback panic, rolls the attempt back and returns an error wrapping [ErrPanicked], as
	// store/mongo does. Otherwise the panic propagates.
	RecoverPanics bool
	// Isolate lists the stores snapshotted before every attempt and restored after every aborted attempt.
	Isolate []Snapshotter

	mu            sync.Mutex
	calls         atomic.Int64
	txns          atomic.Int64
	commitRetries atomic.Int64
}

// innermostTxKey keys the transaction of the innermost running attempt of any Fake, as a driver keeps one session per context.
type innermostTxKey struct{}

// fakeTx is the transaction state of one attempt, stored in its callback context. It is active only while that attempt runs, so
// a context retained after the attempt ended — committed, rolled back, retried or panicked — is no longer in a transaction, as
// store/mongo.InTransaction reports for a finished session.
type fakeTx struct {
	owner  *Fake
	active atomic.Bool
}

// WithTransaction implements [uow.Committer].
func (f *Fake) WithTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	if f.InTransaction(ctx) {
		return fn(ctx)
	}
	f.mu.Lock()
	defer f.mu.Unlock()

	txn := int(f.txns.Add(1))
	attempts := max(f.Attempts, 1)
	for attempt := 1; attempt <= attempts; attempt++ {
		done, err := f.attempt(ctx, fn, txn, attempt == attempts)
		if done {
			return err
		}
	}
	return nil
}

// attempt runs one attempt of transaction txn in its own transaction state, deactivated on every exit (panics included). It
// reports done when the transaction ended with err; otherwise the attempt was rolled back for a retry.
func (f *Fake) attempt(ctx context.Context, fn func(ctx context.Context) error, txn int, last bool) (done bool, err error) {
	tx := &fakeTx{owner: f}
	tx.active.Store(true)
	defer tx.active.Store(false)
	tctx := context.WithValue(ctx, innermostTxKey{}, tx)

	rollback := f.snapshot()
	f.calls.Add(1)
	if err := f.call(tctx, fn, rollback); err != nil {
		rollback()
		return true, err
	}
	if !last {
		rollback()
		return false, nil
	}
	f.commitRetries.Add(int64(max(f.CommitRetries, 0)))
	if f.Abort != nil {
		if err := f.Abort(txn); err != nil {
			rollback()
			return true, err
		}
	}
	if f.Fail != nil {
		rollback()
		return true, f.Fail
	}
	if f.Unknown != nil {
		if err := f.Unknown(txn); err != nil {
			return true, err
		}
	}
	return true, nil
}

// snapshot captures every isolated store and returns the function that restores them in reverse order.
func (f *Fake) snapshot() (rollback func()) {
	restores := make([]func(), len(f.Isolate))
	for i, s := range f.Isolate {
		restores[i] = s.Snapshot()
	}
	return func() {
		for _, restore := range slices.Backward(restores) {
			if restore != nil {
				restore()
			}
		}
	}
}

// call runs fn, converting a panic into an [ErrPanicked] error when RecoverPanics is set (the caller then rolls the attempt
// back). Otherwise a panic rolls the attempt back here and propagates, so writes of a panicking callback never stay visible.
func (f *Fake) call(ctx context.Context, fn func(ctx context.Context) error, rollback func()) (err error) {
	if f.RecoverPanics {
		defer panics.HandleWithOpts(ctx, panics.NewHandleOpts().SetReallyPanic(false), func(_ context.Context, r any) {
			err = fmt.Errorf("%w: %v", ErrPanicked, r)
		})
		return fn(ctx)
	}
	defer func() {
		if r := recover(); r != nil {
			rollback()
			panic(r)
		}
	}()
	return fn(ctx)
}

// InTransaction reports whether the innermost running callback attempt in ctx is one of f, which WithTransaction joins, as
// store/mongo joins a running transaction of the same client only: inside another Fake's attempt started within f's, f starts a
// transaction of its own. A context kept after its attempt ended reports false.
func (f *Fake) InTransaction(ctx context.Context) bool {
	tx, ok := Transaction(ctx)
	if !ok {
		return false
	}
	t, isTx := tx.(*fakeTx)
	return isTx && t.owner == f
}

// Calls returns the number of callback invocations on f.
func (f *Fake) Calls() int {
	return int(f.calls.Load())
}

// Txns returns the number of root WithTransaction calls on f.
func (f *Fake) Txns() int {
	return int(f.txns.Load())
}

// CommitRetriesDone returns the number of commit-only retries executed on f.
func (f *Fake) CommitRetriesDone() int {
	return int(f.commitRetries.Load())
}

// CommitAttempts returns the total number of commit attempts (root transactions plus commit retries) executed on f.
func (f *Fake) CommitAttempts() int {
	return int(f.txns.Load() + f.commitRetries.Load())
}

// Transaction reports the transaction of the innermost running callback attempt of any Fake in ctx, as a driver reports the
// session a context carries; it satisfies [uow.Detector]. A Fake started inside another Fake's attempt is a different transaction
// (another client's); a joined call keeps the enclosing one. A context kept after its attempt ended reports none.
func Transaction(ctx context.Context) (tx any, ok bool) {
	if ctx == nil {
		return nil, false
	}
	t, isTx := ctx.Value(innermostTxKey{}).(*fakeTx)
	if !isTx || !t.active.Load() {
		return nil, false
	}
	return t, true
}
