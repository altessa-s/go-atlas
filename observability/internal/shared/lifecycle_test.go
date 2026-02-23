// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package shared

import (
	"context"
	"errors"
	"testing"
)

type mockShutdownable struct {
	err error
}

func (m *mockShutdownable) Shutdown(_ context.Context) error { return m.err }

type mockFlushable struct {
	err error
}

func (m *mockFlushable) ForceFlush(_ context.Context) error { return m.err }

func TestMultiShutdown(t *testing.T) {
	tests := []struct {
		name       string
		components []Shutdownable
		wantErr    bool
	}{
		{"nil", nil, false},
		{"no errors", []Shutdownable{&mockShutdownable{}, &mockShutdownable{}}, false},
		{"with nil", []Shutdownable{nil, &mockShutdownable{}}, false},
		{"one error", []Shutdownable{&mockShutdownable{err: errors.New("fail")}}, true},
		{"multi error", []Shutdownable{
			&mockShutdownable{err: errors.New("a")},
			&mockShutdownable{err: errors.New("b")},
		}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := MultiShutdown(t.Context(), tt.components...)
			if (err != nil) != tt.wantErr {
				t.Errorf("MultiShutdown() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMultiFlush(t *testing.T) {
	tests := []struct {
		name       string
		components []Flushable
		wantErr    bool
	}{
		{"nil", nil, false},
		{"no errors", []Flushable{&mockFlushable{}, &mockFlushable{}}, false},
		{"with nil", []Flushable{nil, &mockFlushable{}}, false},
		{"one error", []Flushable{&mockFlushable{err: errors.New("fail")}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := MultiFlush(t.Context(), tt.components...)
			if (err != nil) != tt.wantErr {
				t.Errorf("MultiFlush() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSentinelErrors(t *testing.T) {
	if ErrShutdown == nil {
		t.Error("ErrShutdown should not be nil")
	}
	if ErrAlreadyShutdown == nil {
		t.Error("ErrAlreadyShutdown should not be nil")
	}
}
