// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outbox

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// poisonHandler returns a transient-looking error on every invocation
// and counts how many times it was called. It models the "poison
// message" the bounded-retry fix is designed to contain.
func poisonHandler(counter *atomic.Int64) Handler {
	return func(_ context.Context, _ Event) error {
		counter.Add(1)
		return errors.New("transient downstream failure")
	}
}

// TestDispatchEvent_BoundedByRetryMaxAttempts is the regression guard
// for the infinite-retry fix. Before the fix, dispatchEvent used
// WithMaxAttempts(-1) and a poison message kept retrying until the
// context cancelled, starving a worker slot in concurrency.ProcessCollect
// indefinitely. Now the per-event retry budget MUST match the
// configured retryMaxAttempts so the loop terminates and the failure
// escalates to the outbox state machine for the next cycle.
func TestDispatchEvent_BoundedByRetryMaxAttempts(t *testing.T) {
	const attempts = 3

	var calls atomic.Int64
	ob := New(&deadlineCapturingStore{deadlineCh: make(chan time.Duration, 1)},
		poisonHandler(&calls),
		WithRetryMaxAttempts(attempts),
	)

	// Give the test a generous overall deadline — the hardcoded
	// DefaultDispatchRetryBaseDelay is 500ms with exponential backoff,
	// so 3 attempts complete well under 5s.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	err := ob.dispatchEvent(ctx, Event{Id: "poison-1"})

	require.Error(t, err, "dispatchEvent must surface the handler error after exhausting the retry budget")
	// coreretry's WithMaxAttempts counts RETRIES on top of the initial
	// invocation (loop condition: attempt <= maxAttempts, starting at
	// 0). So `attempts=3` means 1 initial + 3 retries = 4 calls. What
	// the test asserts is the bounded property — not infinite.
	require.Equal(t, int64(attempts+1), calls.Load(),
		"handler must be invoked exactly retryMaxAttempts+1 times — bounded retry, not infinite")
}

// TestDispatchEvent_ContextCancellationStillStops keeps the pre-fix
// behavior for explicit ctx cancellation: the loop must abort early
// rather than blocking on the next backoff. Validates that bounding the
// budget did not regress the "ctx wins" property.
func TestDispatchEvent_ContextCancellationStillStops(t *testing.T) {
	var calls atomic.Int64
	ob := New(&deadlineCapturingStore{deadlineCh: make(chan time.Duration, 1)},
		poisonHandler(&calls),
		WithRetryMaxAttempts(1_000), // high — we want ctx cancel to win
	)

	// Cancel before DefaultDispatchRetryBaseDelay (500ms) elapses so
	// the second attempt never fires.
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()

	err := ob.dispatchEvent(ctx, Event{Id: "ctx-cancel"})
	require.Error(t, err)
	require.Less(t, calls.Load(), int64(5),
		"ctx cancellation must abort the retry loop quickly — not wait for the full attempt budget")
}
