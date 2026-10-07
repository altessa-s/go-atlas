// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package concurrency_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/runtime/concurrency"
)

func TestProcess_Success(t *testing.T) {
	var count atomic.Int32
	items := []int{1, 2, 3, 4, 5}

	err := concurrency.Process(t.Context(), items, func(ctx context.Context, item int) error {
		count.Add(1)
		return nil
	}, concurrency.WithConcurrency[int](2))

	require.NoError(t, err)
	require.Equal(t, int32(5), count.Load())
}

func TestProcess_StopOnError(t *testing.T) {
	wantErr := errors.New("fail")

	err := concurrency.Process(t.Context(), []int{1, 2, 3}, func(ctx context.Context, item int) error {
		if item == 2 {
			return wantErr
		}
		return nil
	}, concurrency.WithConcurrency[int](1), concurrency.WithStopOnError[int]())

	require.Error(t, err, "Process() expected error")
}

func TestProcess_OnSuccessOnError(t *testing.T) {
	var successCount, errorCount atomic.Int32

	_ = concurrency.Process(t.Context(), []int{1, 2, 3}, func(ctx context.Context, item int) error {
		if item == 2 {
			return errors.New("fail")
		}
		return nil
	},
		concurrency.WithConcurrency[int](1),
		concurrency.WithOnSuccess[int](func(item int) { successCount.Add(1) }),
		concurrency.WithOnError[int](func(item int, err error) { errorCount.Add(1) }),
	)

	require.Equal(t, int32(1), errorCount.Load())
	require.GreaterOrEqual(t, successCount.Load(), int32(1))
}

func TestProcess_Sequential_ContinueOnError_ProcessesAllItems(t *testing.T) {
	wantErr := errors.New("fail")
	var processed atomic.Int32

	err := concurrency.Process(t.Context(), []int{1, 2, 3}, func(_ context.Context, item int) error {
		processed.Add(1)
		if item == 2 {
			return wantErr
		}
		return nil
	}, concurrency.WithConcurrency[int](1))

	require.ErrorIs(t, err, wantErr)
	require.Equal(t, int32(3), processed.Load(),
		"without WithStopOnError the sequential path must process every item")
}

func TestProcess_Sequential_StopOnError_StopsAtFirstError(t *testing.T) {
	wantErr := errors.New("fail")
	var processed atomic.Int32

	err := concurrency.Process(t.Context(), []int{1, 2, 3}, func(_ context.Context, item int) error {
		processed.Add(1)
		if item == 1 {
			return wantErr
		}
		return nil
	}, concurrency.WithConcurrency[int](1), concurrency.WithStopOnError[int]())

	require.ErrorIs(t, err, wantErr)
	require.Equal(t, int32(1), processed.Load(),
		"with WithStopOnError the sequential path must stop at the first error")
}

func TestProcess_EmptyItems(t *testing.T) {
	err := concurrency.Process(t.Context(), []int{}, func(ctx context.Context, item int) error {
		t.Error("should not be called")
		return nil
	}, concurrency.WithConcurrency[int](2))

	require.NoError(t, err)
}

func TestProcess_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	// With canceled context, Process may or may not return an error
	// depending on timing. Just ensure it doesn't panic.
	_ = concurrency.Process(ctx, []int{1, 2, 3}, func(ctx context.Context, item int) error {
		return ctx.Err()
	}, concurrency.WithConcurrency[int](2), concurrency.WithStopOnError[int]())
}

// A canceled context must never read as a completed batch: no callback runs on
// a pre-canceled context, the error is context.Canceled, and ProcessCollect
// returns no zero values for items it never processed.
func TestProcess_PreCanceledContext_ReportsCancellation(t *testing.T) {
	t.Parallel()
	for _, workers := range []int{1, 2} {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		var calls atomic.Int32

		err := concurrency.Process(ctx, []int{1, 2}, func(context.Context, int) error {
			calls.Add(1)
			return nil
		}, concurrency.WithConcurrency[int](workers))
		require.ErrorIs(t, err, context.Canceled, "workers=%d", workers)

		results, err := concurrency.ProcessCollect(ctx, []int{1, 2}, func(_ context.Context, n int) (int, error) {
			calls.Add(1)
			return n * 10, nil
		}, concurrency.WithConcurrency[int](workers))
		require.ErrorIs(t, err, context.Canceled, "workers=%d", workers)
		require.Empty(t, results, "workers=%d: no fabricated results", workers)
		require.Zero(t, calls.Load(), "workers=%d: no callback on a canceled context", workers)
	}
}

// Canceling mid-batch reports the cancellation and leaves the rest
// unprocessed; canceling during the final callback, after every item was
// processed, is a completed batch.
func TestProcess_Sequential_CancellationSemantics(t *testing.T) {
	t.Parallel()

	t.Run("mid_batch", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(t.Context())
		var calls atomic.Int32
		err := concurrency.Process(ctx, []int{1, 2, 3}, func(context.Context, int) error {
			if calls.Add(1) == 1 {
				cancel()
			}
			return nil
		}, concurrency.WithConcurrency[int](1))
		require.ErrorIs(t, err, context.Canceled)
		require.Equal(t, int32(1), calls.Load())
	})

	t.Run("during_last_item", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(t.Context())
		var calls atomic.Int32
		err := concurrency.Process(ctx, []int{1, 2}, func(context.Context, int) error {
			if calls.Add(1) == 2 {
				cancel()
			}
			return nil
		}, concurrency.WithConcurrency[int](1))
		require.NoError(t, err, "every item was processed")
		require.Equal(t, int32(2), calls.Load())
	})
}

func TestProcessCollect_Error(t *testing.T) {
	wantErr := errors.New("transform fail")

	_, err := concurrency.ProcessCollect(t.Context(), []int{1, 2}, func(ctx context.Context, item int) (string, error) {
		if item == 2 {
			return "", wantErr
		}
		return "ok", nil
	}, concurrency.WithConcurrency[int](1), concurrency.WithStopOnError[int]())

	require.Error(t, err, "ProcessCollect() expected error")
}

func TestDefaultLimitFunc(t *testing.T) {
	limit := concurrency.DefaultLimitFunc()
	require.GreaterOrEqual(t, limit, 1)
}

func TestProcess_WithLimitFunc(t *testing.T) {
	var count atomic.Int32

	err := concurrency.Process(t.Context(), []int{1, 2, 3}, func(ctx context.Context, item int) error {
		count.Add(1)
		return nil
	}, concurrency.WithLimitFunc[int](func() int { return 2 }))

	require.NoError(t, err)
	require.Equal(t, int32(3), count.Load())
}
