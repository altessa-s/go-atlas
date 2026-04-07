// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package plugins

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errSymbolNotFound mimics the error plugin.Plugin.Lookup returns for missing
// symbols. Its exact type is unimportant for the helper under test.
var errSymbolNotFound = errors.New("symbol not found")

// stubLookup builds a symbolLookup that returns canned values for a set of
// symbol names. Absent names return errSymbolNotFound. It is a stub in the
// Google test-double sense — no logic, only pre-registered responses.
func stubLookup(symbols map[string]any) symbolLookup {
	return func(name string) (any, error) {
		if sym, ok := symbols[name]; ok {
			return sym, nil
		}
		return nil, errSymbolNotFound
	}
}

func TestResolveDescriptor(t *testing.T) {
	valueForm := &Descriptor{Name: "value-plugin", Version: "1.0.0"}
	pointerForm := &Descriptor{Name: "pointer-plugin", Version: "2.0.0"}
	pointerFormPtr := &pointerForm

	tests := []struct {
		name    string
		symbols map[string]any
		wantErr error
		wantFn  func(t *testing.T, got *Descriptor)
	}{
		{
			name:    "value declaration form (*Descriptor)",
			symbols: map[string]any{"Descriptor": valueForm},
			wantFn: func(t *testing.T, got *Descriptor) {
				assert.Equal(t, "value-plugin", got.Name)
				assert.Equal(t, "1.0.0", got.Version)
			},
		},
		{
			name:    "pointer declaration form (**Descriptor)",
			symbols: map[string]any{"Descriptor": pointerFormPtr},
			wantFn: func(t *testing.T, got *Descriptor) {
				assert.Equal(t, "pointer-plugin", got.Name)
				assert.Equal(t, "2.0.0", got.Version)
			},
		},
		{
			name:    "absent symbol returns ErrNoDescriptor",
			symbols: map[string]any{},
			wantErr: ErrNoDescriptor,
		},
		{
			name:    "wrong type returns ErrInvalidDescriptor",
			symbols: map[string]any{"Descriptor": "not a descriptor"},
			wantErr: ErrInvalidDescriptor,
		},
		{
			name:    "nil *Descriptor returns ErrInvalidDescriptor",
			symbols: map[string]any{"Descriptor": (*Descriptor)(nil)},
			wantErr: ErrInvalidDescriptor,
		},
		{
			name:    "nil **Descriptor returns ErrInvalidDescriptor",
			symbols: map[string]any{"Descriptor": (**Descriptor)(nil)},
			wantErr: ErrInvalidDescriptor,
		},
		{
			name:    "empty Name returns ErrInvalidDescriptor",
			symbols: map[string]any{"Descriptor": &Descriptor{Name: "", Version: "1.0.0"}},
			wantErr: ErrInvalidDescriptor,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveDescriptor(stubLookup(tc.symbols))
			if tc.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tc.wantErr)
				assert.Nil(t, got)
				return
			}
			require.NoError(t, err)
			tc.wantFn(t, got)
		})
	}
}

func TestResolveInit(t *testing.T) {
	// Each test case carries its own setup closure that returns a fresh
	// symbol table plus an `invoked` predicate. This isolates state per
	// subtest — no shared counters to reset, parallel-safe by construction.
	type initCase struct {
		name    string
		setup   func() (symbols map[string]any, invoked func() bool)
		wantNil bool
		wantErr bool
	}

	cases := []initCase{
		{
			name: "function declaration form",
			setup: func() (map[string]any, func() bool) {
				called := false
				// Function declaration: `func Init(ctx) error { ... }`
				// plugin.Lookup returns func(ctx) error directly.
				fn := func(_ context.Context) error {
					called = true
					return nil
				}
				return map[string]any{"Init": fn}, func() bool { return called }
			},
		},
		{
			name: "variable declaration form",
			setup: func() (map[string]any, func() bool) {
				called := false
				// Variable form: `var Init = func(ctx) error { ... }`
				// plugin.Lookup returns *func(ctx) error.
				fn := func(_ context.Context) error {
					called = true
					return nil
				}
				return map[string]any{"Init": &fn}, func() bool { return called }
			},
		},
		{
			name:    "absent symbol returns (nil, nil)",
			setup:   func() (map[string]any, func() bool) { return map[string]any{}, nil },
			wantNil: true,
		},
		{
			name: "nil *func(ctx) error returns (nil, nil)",
			setup: func() (map[string]any, func() bool) {
				return map[string]any{"Init": (*func(context.Context) error)(nil)}, nil
			},
			wantNil: true,
		},
		{
			name: "wrong type returns error",
			setup: func() (map[string]any, func() bool) {
				return map[string]any{"Init": "not a function"}, nil
			},
			wantErr: true,
		},
		{
			name: "wrong signature returns error",
			setup: func() (map[string]any, func() bool) {
				return map[string]any{"Init": func() error { return nil }}, nil
			},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			symbols, invoked := tc.setup()
			fn, err := resolveInit(stubLookup(symbols))
			if tc.wantErr {
				require.Error(t, err)
				assert.Nil(t, fn)
				return
			}
			require.NoError(t, err)
			if tc.wantNil {
				assert.Nil(t, fn)
				return
			}
			require.NotNil(t, fn)
			require.NoError(t, fn(t.Context()))
			assert.True(t, invoked(), "Init function was not invoked")
		})
	}
}
