// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outbox

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type deadlineCapturingStore struct {
	deadlineCh chan time.Duration
}

func (s *deadlineCapturingStore) FetchUnprocessedEvents(ctx context.Context, _ uint32, _ time.Time) ([]Event, error) {
	if dl, ok := ctx.Deadline(); ok {
		// Report remaining time until deadline at the moment of the call.
		select {
		case s.deadlineCh <- time.Until(dl):
		default:
		}
	}
	return nil, nil
}

func (s *deadlineCapturingStore) DeleteProcessedEvents(context.Context, time.Time) error { return nil }
func (s *deadlineCapturingStore) UnlockStuckEvents(context.Context, time.Time) error     { return nil }
func (s *deadlineCapturingStore) SaveEvents(context.Context, ...Event) error             { return nil }
func (s *deadlineCapturingStore) UpdateEvents(context.Context, ...Event) error           { return nil }
func (s *deadlineCapturingStore) ExpireEvents(context.Context, time.Time) (int64, error) {
	return 0, nil
}

func TestOutbox_WithFetchTimeout_AppliesDeadlineToFetch(t *testing.T) {
	const (
		fetchTimeout = 50 * time.Millisecond
		waitTimeout  = 2 * time.Second
		margin       = 100 * time.Millisecond
	)

	s := &deadlineCapturingStore{
		deadlineCh: make(chan time.Duration, 1),
	}

	ob := New(s, noopHandler,
		WithFetchTimeout(fetchTimeout),
	)

	require.NoError(t, ob.RunDispatchCycle(t.Context()))

	select {
	case remaining := <-s.deadlineCh:
		require.True(t, remaining > 0, "expected fetch ctx to have a future deadline; got remaining=%s", remaining)
		require.True(t, remaining <= fetchTimeout+margin, "expected fetch ctx deadline to be within %s (+%s); got remaining=%s", fetchTimeout, margin, remaining)
	default:
		require.Fail(t, "expected FetchUnprocessedEvents to be called")
	}
}
