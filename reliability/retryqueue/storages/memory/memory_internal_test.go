// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/reliability/retryqueue"
)

// A call whose context ends while it waits for the mutex fails with the
// context error once it gets the mutex, and changes nothing. The test holds
// the mutex itself and uses the beforeLock seam (white-box) to cancel the
// context only after the call passed the first check and is about to block,
// so only the check made after acquiring the mutex can refuse it.
func TestCanceledWhileWaitingForLock(t *testing.T) {
	t.Parallel()

	t0 := time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)
	calls := map[string]func(context.Context, *Store[string]) error{
		"Mark": func(ctx context.Context, s *Store[string]) error {
			_, err := s.Mark(ctx, retryqueue.Item[string]{Key: "new", Since: t0, NextAttemptAt: t0})
			return err
		},
		"Reopen": func(ctx context.Context, s *Store[string]) error {
			_, err := s.Reopen(ctx, retryqueue.Item[string]{Key: "k", Generation: 2, Since: t0, NextAttemptAt: t0})
			return err
		},
		"ListDue":        func(ctx context.Context, s *Store[string]) error { _, err := s.ListDue(ctx, t0, 0); return err },
		"WakeDue":        func(ctx context.Context, s *Store[string]) error { return s.WakeDue(ctx, t0.Add(-time.Hour)) },
		"Reschedule":     func(ctx context.Context, s *Store[string]) error { return s.Reschedule(ctx, "k", 1, t0.Add(time.Hour)) },
		"Expire":         func(ctx context.Context, s *Store[string]) error { _, err := s.Expire(ctx, "k", 1, t0, t0); return err },
		"Resolve":        func(ctx context.Context, s *Store[string]) error { return s.Resolve(ctx, "k", 1, t0) },
		"Flag":           func(ctx context.Context, s *Store[string]) error { _, err := s.Flag(ctx, "k", 1, "why"); return err },
		"ClearFlag":      func(ctx context.Context, s *Store[string]) error { return s.ClearFlag(ctx, "k", 1) },
		"DeleteObsolete": func(ctx context.Context, s *Store[string]) error { _, err := s.DeleteObsolete(ctx, "k", 1); return err },
		"Delete":         func(ctx context.Context, s *Store[string]) error { return s.Delete(ctx, "k") },
		"ListByState": func(ctx context.Context, s *Store[string]) error {
			_, err := s.ListByState(ctx, retryqueue.StatePending, 0)
			return err
		},
		"Requeue": func(ctx context.Context, s *Store[string]) error { _, err := s.Requeue(ctx, t0); return err },
		"Move":    func(ctx context.Context, s *Store[string]) error { return s.Move(ctx, "k", "moved") },
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			s := New[string]()
			_, err := s.Mark(t.Context(), retryqueue.Item[string]{Key: "k", Generation: 1, Since: t0, NextAttemptAt: t0, Payload: "p"})
			require.NoError(t, err)
			before := s.items["k"]

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			waiting := make(chan struct{})
			s.beforeLock = func() { close(waiting) }

			s.mu.Lock()
			done := make(chan error, 1)
			go func() { done <- call(ctx, s) }()
			<-waiting // the call passed the first check and is about to block on mu
			cancel()
			s.mu.Unlock()

			require.ErrorIs(t, <-done, context.Canceled)
			s.beforeLock = nil
			require.Equal(t, 1, s.Len())
			require.Equal(t, before, s.items["k"])
		})
	}
}
