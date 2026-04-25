// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package budget_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/limiters/budget"
	"github.com/altessa-s/go-atlas/data/limiters/storages/memory"
)

func validSettings() *budget.Settings {
	return &budget.Settings{Limit: 10, Period: time.Minute}
}

func TestNew_NilSettings(t *testing.T) {
	_, err := budget.New(nil, memory.New())
	require.Error(t, err, "New(nil, ...) should return error")
}

func TestNew_InvalidSettings(t *testing.T) {
	_, err := budget.New(&budget.Settings{}, memory.New())
	require.Error(t, err, "New(invalid, ...) should return error")
}

func TestNew_NilStorage(t *testing.T) {
	_, err := budget.New(validSettings(), nil)
	require.Error(t, err, "New(..., nil) should return error")
}

func TestNew_Valid(t *testing.T) {
	l, err := budget.New(validSettings(), memory.New())
	require.NoError(t, err)
	require.NotNil(t, l, "New() returned nil")
}

func TestAllow_WithinBudget(t *testing.T) {
	l, err := budget.New(validSettings(), memory.New())
	require.NoError(t, err)

	require.NoError(t, l.Allow(t.Context(), "key-1"))
}

func TestAllow_BudgetExhausted(t *testing.T) {
	cfg := &budget.Settings{Limit: 1, Period: time.Minute}
	l, err := budget.New(cfg, memory.New())
	require.NoError(t, err)

	// First request should succeed.
	require.NoError(t, l.Allow(t.Context(), "key-1"))

	// Second request should exhaust the budget.
	err = l.Allow(t.Context(), "key-1")
	require.ErrorIs(t, err, budget.ErrBudgetExhausted)
}

func TestAllow_SeparateKeys(t *testing.T) {
	cfg := &budget.Settings{Limit: 1, Period: time.Minute}
	l, err := budget.New(cfg, memory.New())
	require.NoError(t, err)

	require.NoError(t, l.Allow(t.Context(), "key-a"))

	// Different key has its own budget.
	require.NoError(t, l.Allow(t.Context(), "key-b"))
}

func TestSettings_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     budget.Settings
		wantErr bool
	}{
		{"valid", budget.Settings{Limit: 100, Period: time.Hour}, false},
		{"zero limit", budget.Settings{Limit: 0, Period: time.Hour}, true},
		{"negative limit", budget.Settings{Limit: -1, Period: time.Hour}, true},
		{"zero period", budget.Settings{Limit: 100, Period: 0}, true},
		{"sub-second period", budget.Settings{Limit: 100, Period: 500 * time.Millisecond}, true},
		{"min valid period", budget.Settings{Limit: 1, Period: time.Second}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
