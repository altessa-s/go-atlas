// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package budget_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/coord/limiters/budget"
	"github.com/altessa-s/go-atlas/coord/limiters/storages"
	"github.com/altessa-s/go-atlas/coord/limiters/storages/memory"
)

var (
	errCustomPeriod = errors.New("period exceeds custom backend retention")
	errCustomLimit  = errors.New("limit exceeds custom backend cap")
)

type validatingStorage struct {
	maxPeriod time.Duration
	maxLimit  int64
}

func (s *validatingStorage) Allow(context.Context, string, int64, time.Duration) (*storages.LimitInfo, error) {
	return &storages.LimitInfo{}, nil
}

func (s *validatingStorage) Reset(context.Context, string) error { return nil }

func (s *validatingStorage) ValidatePeriod(period time.Duration) error {
	if s.maxPeriod > 0 && period > s.maxPeriod {
		return fmt.Errorf("custom storage: %w", errCustomPeriod)
	}
	return nil
}

func (s *validatingStorage) ValidateLimit(limit int64) error {
	if s.maxLimit > 0 && limit > s.maxLimit {
		return fmt.Errorf("custom storage: %w", errCustomLimit)
	}
	return nil
}

func TestNew_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		limit   int64
		period  time.Duration
		storage storages.Storage
		wantErr error
	}{
		{"valid", 100, time.Hour, memory.New(), nil},
		{"min valid period", 1, budget.MinPeriod, memory.New(), nil},
		{"zero limit", 0, time.Hour, memory.New(), budget.ErrInvalidLimit},
		{"negative limit", -1, time.Hour, memory.New(), budget.ErrInvalidLimit},
		{"zero period", 100, 0, memory.New(), budget.ErrInvalidPeriod},
		{"sub-second period", 100, 500 * time.Millisecond, memory.New(), budget.ErrInvalidPeriod},
		{"nil storage", 100, time.Hour, nil, budget.ErrNilStorage},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			l, err := budget.New(tt.limit, tt.period, tt.storage)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				require.Nil(t, l)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, l)
		})
	}
}

func TestAllow_WithinBudget(t *testing.T) {
	t.Parallel()

	l, err := budget.New(10, time.Minute, memory.New())
	require.NoError(t, err)

	require.NoError(t, l.Allow(t.Context(), "key-1"))
}

func TestAllow_BudgetExhausted(t *testing.T) {
	t.Parallel()

	l, err := budget.New(1, time.Minute, memory.New())
	require.NoError(t, err)

	// First request should succeed.
	require.NoError(t, l.Allow(t.Context(), "key-1"))

	// Second request should exhaust the budget.
	err = l.Allow(t.Context(), "key-1")
	require.ErrorIs(t, err, budget.ErrBudgetExhausted)
}

func TestAllow_SeparateKeys(t *testing.T) {
	t.Parallel()

	l, err := budget.New(1, time.Minute, memory.New())
	require.NoError(t, err)

	require.NoError(t, l.Allow(t.Context(), "key-a"))

	// Different key has its own budget.
	require.NoError(t, l.Allow(t.Context(), "key-b"))
}

func TestNew_CustomStorageValidators(t *testing.T) {
	t.Parallel()

	store := &validatingStorage{maxPeriod: time.Hour, maxLimit: 500}

	tests := []struct {
		name    string
		limit   int64
		period  time.Duration
		wantErr error
	}{
		{name: "within bounds", limit: 500, period: time.Hour},
		{name: "period rejected by storage", limit: 100, period: 2 * time.Hour, wantErr: errCustomPeriod},
		{name: "limit rejected by storage", limit: 501, period: time.Hour, wantErr: errCustomLimit},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			l, err := budget.New(tc.limit, tc.period, store)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				require.Nil(t, l)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, l)
		})
	}
}
