// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package limiters

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
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
			got := li.IsLimitExceeded()
			require.Equal(t, tt.want, got)
		})
	}
}

func TestFunc_Limit(t *testing.T) {
	expected := &LimitInfo{Limit: 100, Remaining: 50, Reset: 1234567890}
	fn := Func(func(_ context.Context) (*LimitInfo, error) {
		return expected, nil
	})

	info, err := fn.Limit(t.Context())
	require.NoError(t, err)
	require.Equal(t, expected, info)
}

func TestFunc_Limit_Error(t *testing.T) {
	fn := Func(func(_ context.Context) (*LimitInfo, error) {
		return nil, ErrLimitExceeded
	})

	_, err := fn.Limit(t.Context())
	require.ErrorIs(t, err, ErrLimitExceeded)
}
