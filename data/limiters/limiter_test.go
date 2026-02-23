// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package limiters

import (
	"context"
	"errors"
	"testing"
)

func TestLimitInfo_IsLimitExceeded(t *testing.T) {
	tests := []struct {
		name      string
		remaining int64
		want      bool
	}{
		{"positive_remaining", 5, false},
		{"zero_remaining", 0, true},
		{"negative_remaining", -1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			li := &LimitInfo{Remaining: tt.remaining}
			if got := li.IsLimitExceeded(); got != tt.want {
				t.Fatalf("IsLimitExceeded() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestErrLimitExceeded(t *testing.T) {
	if ErrLimitExceeded == nil {
		t.Fatal("ErrLimitExceeded is nil")
	}
	if ErrLimitExceeded.Error() != "rate limit exceeded" {
		t.Fatalf("ErrLimitExceeded.Error() = %q", ErrLimitExceeded.Error())
	}
}

func TestFunc_Limit(t *testing.T) {
	expected := &LimitInfo{Limit: 100, Remaining: 50, Reset: 1234567890}
	fn := Func(func(_ context.Context) (*LimitInfo, error) {
		return expected, nil
	})

	info, err := fn.Limit(t.Context())
	if err != nil {
		t.Fatalf("Limit() error = %v", err)
	}
	if info != expected {
		t.Fatalf("Limit() = %v, want %v", info, expected)
	}
}

func TestFunc_Limit_Error(t *testing.T) {
	fn := Func(func(_ context.Context) (*LimitInfo, error) {
		return nil, ErrLimitExceeded
	})

	_, err := fn.Limit(t.Context())
	if !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("Limit() error = %v, want ErrLimitExceeded", err)
	}
}
