// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package budget_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/limiters/budget"
	"github.com/altessa-s/go-atlas/data/limiters/storages"
	"github.com/altessa-s/go-atlas/data/limiters/storages/memory"
)

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
