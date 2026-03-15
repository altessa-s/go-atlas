// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package budget_test

import (
	"errors"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/data/limiters/budget"
	"github.com/altessa-s/go-atlas/data/limiters/storages/memory"
)

func validSettings() *budget.Settings {
	return &budget.Settings{Limit: 10, Period: time.Minute}
}

func TestNew_NilSettings(t *testing.T) {
	_, err := budget.New(nil, memory.New())
	if err == nil {
		t.Error("New(nil, ...) should return error")
	}
}

func TestNew_InvalidSettings(t *testing.T) {
	_, err := budget.New(&budget.Settings{}, memory.New())
	if err == nil {
		t.Error("New(invalid, ...) should return error")
	}
}

func TestNew_NilStorage(t *testing.T) {
	_, err := budget.New(validSettings(), nil)
	if err == nil {
		t.Error("New(..., nil) should return error")
	}
}

func TestNew_Valid(t *testing.T) {
	l, err := budget.New(validSettings(), memory.New())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if l == nil {
		t.Fatal("New() returned nil")
	}
}

func TestAllow_WithinBudget(t *testing.T) {
	l, err := budget.New(validSettings(), memory.New())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := l.Allow(t.Context(), "key-1"); err != nil {
		t.Fatalf("Allow() error = %v", err)
	}
}

func TestAllow_BudgetExhausted(t *testing.T) {
	cfg := &budget.Settings{Limit: 1, Period: time.Minute}
	l, err := budget.New(cfg, memory.New())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// First request should succeed.
	if err := l.Allow(t.Context(), "key-1"); err != nil {
		t.Fatalf("first Allow() error = %v", err)
	}

	// Second request should exhaust the budget.
	err = l.Allow(t.Context(), "key-1")
	if !errors.Is(err, budget.ErrBudgetExhausted) {
		t.Errorf("second Allow() error = %v, want ErrBudgetExhausted", err)
	}
}

func TestAllow_SeparateKeys(t *testing.T) {
	cfg := &budget.Settings{Limit: 1, Period: time.Minute}
	l, err := budget.New(cfg, memory.New())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := l.Allow(t.Context(), "key-a"); err != nil {
		t.Fatalf("Allow(key-a) error = %v", err)
	}

	// Different key has its own budget.
	if err := l.Allow(t.Context(), "key-b"); err != nil {
		t.Fatalf("Allow(key-b) error = %v", err)
	}
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
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
