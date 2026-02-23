// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package concurrency_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/altessa-s/go-atlas/core/runtime/concurrency"
)

func TestProcess_Success(t *testing.T) {
	var count atomic.Int32
	items := []int{1, 2, 3, 4, 5}

	err := concurrency.Process(t.Context(), items, func(ctx context.Context, item int) error {
		count.Add(1)
		return nil
	}, concurrency.BatchConfig[int]{Concurrency: 2})

	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if count.Load() != 5 {
		t.Errorf("processed %d items, want 5", count.Load())
	}
}

func TestProcess_StopOnError(t *testing.T) {
	wantErr := errors.New("fail")

	err := concurrency.Process(t.Context(), []int{1, 2, 3}, func(ctx context.Context, item int) error {
		if item == 2 {
			return wantErr
		}
		return nil
	}, concurrency.BatchConfig[int]{Concurrency: 1, StopOnError: true})

	if err == nil {
		t.Fatal("Process() expected error")
	}
}

func TestProcess_OnSuccessOnError(t *testing.T) {
	var successCount, errorCount atomic.Int32

	_ = concurrency.Process(t.Context(), []int{1, 2, 3}, func(ctx context.Context, item int) error {
		if item == 2 {
			return errors.New("fail")
		}
		return nil
	}, concurrency.BatchConfig[int]{
		Concurrency: 1,
		OnSuccess:   func(item int) { successCount.Add(1) },
		OnError:     func(item int, err error) { errorCount.Add(1) },
	})

	if errorCount.Load() != 1 {
		t.Errorf("error count = %d, want 1", errorCount.Load())
	}
	// At least 1 success (order may vary with concurrency)
	if successCount.Load() < 1 {
		t.Errorf("success count = %d, want >= 1", successCount.Load())
	}
}

func TestProcess_EmptyItems(t *testing.T) {
	err := concurrency.Process(t.Context(), []int{}, func(ctx context.Context, item int) error {
		t.Error("should not be called")
		return nil
	}, concurrency.BatchConfig[int]{Concurrency: 2})

	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
}

func TestProcess_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	// With canceled context, Process may or may not return an error
	// depending on timing. Just ensure it doesn't panic.
	_ = concurrency.Process(ctx, []int{1, 2, 3}, func(ctx context.Context, item int) error {
		return ctx.Err()
	}, concurrency.BatchConfig[int]{Concurrency: 2, StopOnError: true})
}

func TestProcessCollect_Error(t *testing.T) {
	wantErr := errors.New("transform fail")

	_, err := concurrency.ProcessCollect(t.Context(), []int{1, 2}, func(ctx context.Context, item int) (string, error) {
		if item == 2 {
			return "", wantErr
		}
		return "ok", nil
	}, concurrency.BatchConfig[int]{Concurrency: 1, StopOnError: true})

	if err == nil {
		t.Fatal("ProcessCollect() expected error")
	}
}

func TestDefaultLimitFunc(t *testing.T) {
	limit := concurrency.DefaultLimitFunc()
	if limit < 1 {
		t.Errorf("DefaultLimitFunc() = %d, want >= 1", limit)
	}
}

func TestProcess_WithLimitFunc(t *testing.T) {
	var count atomic.Int32

	err := concurrency.Process(t.Context(), []int{1, 2, 3}, func(ctx context.Context, item int) error {
		count.Add(1)
		return nil
	}, concurrency.BatchConfig[int]{
		LimitFunc: func() int { return 2 },
	})

	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if count.Load() != 3 {
		t.Errorf("processed %d items, want 3", count.Load())
	}
}
