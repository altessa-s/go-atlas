// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package purgeloop_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/purgeloop"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

func TestStartOnce(t *testing.T) {
	t.Parallel()
	var active, calls atomic.Int32
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	l := purgeloop.New(time.Millisecond, func(ctx context.Context) (int64, error) {
		active.Add(1)
		defer active.Add(-1)
		calls.Add(1)
		once.Do(func() { close(entered) })
		select {
		case <-release:
		case <-ctx.Done():
		}
		return 0, nil
	}, nil, "test rows")
	t.Cleanup(func() { require.NoError(t, l.Close(context.WithoutCancel(t.Context()))) })
	var starts sync.WaitGroup
	for range 20 {
		starts.Go(l.Start)
	}
	starts.Wait()
	testhelpers.WaitFor(t, time.Second, func() bool { return channelClosed(entered) }, "first pass starts")
	for range 1000 {
		l.Start()
	}
	require.EqualValues(t, 1, active.Load(), "one goroutine executes passes")
	require.EqualValues(t, 1, calls.Load())
	close(release)
	require.NoError(t, l.Close(t.Context()))
}

func TestDisabledAndStoppedBeforeStart(t *testing.T) {
	t.Parallel()
	for _, every := range []time.Duration{-time.Second, 0, time.Millisecond} {
		t.Run(every.String(), func(t *testing.T) {
			t.Parallel()
			var calls atomic.Int32
			l := purgeloop.New(every, func(context.Context) (int64, error) { calls.Add(1); return 0, nil }, nil, "rows")
			if every > 0 {
				require.Nil(t, l.Stop())
			}
			l.Start()
			require.Nil(t, l.Stop(), "disabled and stopped loops never start")
			require.Zero(t, calls.Load())
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			require.NoError(t, l.Close(ctx))
		})
	}
}

func TestCloseConcurrentAndPermanent(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	l := purgeloop.New(time.Millisecond, func(ctx context.Context) (int64, error) {
		calls.Add(1)
		<-ctx.Done()
		return 0, ctx.Err()
	}, nil, "rows")
	l.Start()
	testhelpers.WaitFor(t, time.Second, func() bool { return calls.Load() > 0 }, "pass starts")
	var closes sync.WaitGroup
	errs := make(chan error, 10)
	for range 10 {
		closes.Go(func() { errs <- l.Close(t.Context()) })
	}
	closes.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	done := l.Stop()
	require.NotNil(t, done)
	require.True(t, channelClosed(done))
	l.Start()
	require.Equal(t, done, l.Stop())
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.NoError(t, l.Wait(ctx, done), "finished loop satisfies ended context")
	require.NoError(t, l.Close(ctx))
}

func TestWaitContextError(t *testing.T) {
	t.Parallel()
	l := purgeloop.New(0, nil, nil, "test rows")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.NoError(t, l.Wait(ctx, nil))
	err := l.Wait(ctx, make(chan struct{}))
	require.ErrorIs(t, err, context.Canceled)
	require.Contains(t, err.Error(), "wait for test rows purge to stop")
}

func TestPassPanicAndErrorKeepTicking(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	var logs bytes.Buffer // Read after Close joins the writer.
	boom := errors.New("database failed")
	l := purgeloop.New(time.Millisecond, func(ctx context.Context) (int64, error) {
		switch calls.Add(1) {
		case 1:
			panic("pass panic")
		case 2:
			return 4, boom
		}
		return 0, nil
	}, slog.New(slog.NewTextHandler(&logs, nil)), "test rows")
	l.Start()
	t.Cleanup(func() { require.NoError(t, l.Close(context.WithoutCancel(t.Context()))) })
	testhelpers.WaitFor(t, time.Second, func() bool { return calls.Load() >= 3 }, "panic and error leave loop running")
	require.NoError(t, l.Close(t.Context()))
	require.Contains(t, logs.String(), "test rows purge panicked")
	require.Contains(t, logs.String(), "failed to purge test rows")
	require.Contains(t, logs.String(), "deleted=4")
}

func TestCloseCancelsPassAndBoundsWait(t *testing.T) {
	t.Parallel()
	entered, canceled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	l := purgeloop.New(time.Millisecond, func(ctx context.Context) (int64, error) {
		once.Do(func() { close(entered) })
		<-ctx.Done()
		select {
		case <-canceled:
		default:
			close(canceled)
		}
		<-release
		return 0, ctx.Err()
	}, nil, "rows")
	l.Start()
	testhelpers.WaitFor(t, time.Second, func() bool { return channelClosed(entered) }, "pass starts")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, l.Close(ctx), context.Canceled)
	testhelpers.WaitFor(t, time.Second, func() bool { return channelClosed(canceled) }, "stop cancels pass")
	close(release)
	require.NoError(t, l.Close(t.Context()))
}

func TestDrainShortBatchUsesConsideredCount(t *testing.T) {
	t.Parallel()
	calls := 0
	total, err := purgeloop.Drain(t.Context(), 2, 0, func(context.Context) (int64, int64, error) {
		calls++
		if calls < 3 {
			return 1, 2, nil
		}
		return 1, 1, nil
	})
	require.NoError(t, err)
	require.EqualValues(t, 3, total)
	require.Equal(t, 3, calls)
}

// A full batch that deletes nothing (rows still visible in a REPEATABLE READ
// snapshot after a concurrent delete) must end the pass instead of looping.
func TestDrainStopsWithoutProgress(t *testing.T) {
	t.Parallel()
	calls := 0
	total, err := purgeloop.Drain(t.Context(), 2, 0, func(context.Context) (int64, int64, error) {
		calls++
		if calls == 1 {
			return 2, 2, nil
		}
		return 0, 2, nil
	})
	require.NoError(t, err)
	require.EqualValues(t, 2, total)
	require.Equal(t, 2, calls)
}

func TestDrainErrorIncludesCompletedDeletions(t *testing.T) {
	t.Parallel()
	boom := errors.New("step failed")
	calls := 0
	total, err := purgeloop.Drain(t.Context(), 2, 0, func(context.Context) (int64, int64, error) {
		calls++
		if calls == 1 {
			return 2, 2, nil
		}
		return 1, 2, boom
	})
	require.ErrorIs(t, err, boom)
	require.EqualValues(t, 3, total)
	require.Equal(t, 2, calls)
}

func TestDrainCancellation(t *testing.T) {
	t.Parallel()
	for _, before := range []bool{true, false} {
		t.Run(map[bool]string{true: "before step", false: "between steps"}[before], func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if before {
				cancel()
			}
			calls := 0
			total, err := purgeloop.Drain(ctx, 2, 0, func(context.Context) (int64, int64, error) {
				calls++
				cancel()
				return 2, 2, nil
			})
			require.Same(t, context.Canceled, err, "context error is unwrapped")
			if before {
				require.Zero(t, calls)
				require.Zero(t, total)
			} else {
				require.Equal(t, 1, calls)
				require.EqualValues(t, 2, total)
			}
		})
	}
}

func TestDrainStepTimeout(t *testing.T) {
	t.Parallel()
	for _, timeout := range []time.Duration{-time.Second, 0, time.Second} {
		t.Run(timeout.String(), func(t *testing.T) {
			t.Parallel()
			want := timeout
			if want <= 0 {
				want = purgeloop.DefaultStepTimeout
			}
			ctx := t.Context()
			type valueKey struct{}
			ctx = context.WithValue(ctx, valueKey{}, "transaction")
			before := time.Now()
			var steps []context.Context
			_, err := purgeloop.Drain(ctx, 2, timeout, func(stepCtx context.Context) (int64, int64, error) {
				deadline, ok := stepCtx.Deadline()
				require.True(t, ok)
				require.WithinDuration(t, before.Add(want), deadline, 100*time.Millisecond)
				require.Equal(t, "transaction", stepCtx.Value(valueKey{}))
				for _, previous := range steps {
					require.ErrorIs(t, previous.Err(), context.Canceled)
				}
				steps = append(steps, stepCtx)
				if len(steps) == 1 {
					return 2, 2, nil
				}
				return 0, 0, nil
			})
			require.NoError(t, err)
			require.Len(t, steps, 2)
			require.ErrorIs(t, steps[1].Err(), context.Canceled)
		})
	}
}

func TestDrainStepDeadlineExpires(t *testing.T) {
	t.Parallel()
	total, err := purgeloop.Drain(t.Context(), 2, time.Millisecond, func(ctx context.Context) (int64, int64, error) {
		<-ctx.Done()
		return 0, 0, ctx.Err()
	})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Zero(t, total)
}

func channelClosed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}
