// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tokenbucket_test

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/limiters/tokenbucket"
)

func TestRateLimitSettings_Validate(t *testing.T) {
	tests := []struct {
		name    string
		s       tokenbucket.RateLimitSettings
		wantErr bool
	}{
		{"valid", tokenbucket.RateLimitSettings{Limit: 100, Period: time.Minute}, false},
		{"zero limit", tokenbucket.RateLimitSettings{Limit: 0, Period: time.Minute}, true},
		{"zero period", tokenbucket.RateLimitSettings{Limit: 100, Period: 0}, true},
		{"negative limit", tokenbucket.RateLimitSettings{Limit: -1, Period: time.Minute}, true},
		{"negative period", tokenbucket.RateLimitSettings{Limit: 100, Period: -time.Second}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.s.Validate()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestRateLimitSettings_IsUnlimited(t *testing.T) {
	require.True(t, tokenbucket.RateLimitUnlimited.IsUnlimited(), "RateLimitUnlimited.IsUnlimited() = false, want true")

	normal := &tokenbucket.RateLimitSettings{Limit: 100, Period: time.Minute}
	require.False(t, normal.IsUnlimited(), "normal settings should not be unlimited")
}

func TestRateLimitSettings_IsSkip(t *testing.T) {
	require.True(t, tokenbucket.RateLimitSkip.IsSkip(), "RateLimitSkip.IsSkip() = false, want true")

	var nilSettings *tokenbucket.RateLimitSettings
	require.True(t, nilSettings.IsSkip(), "nil settings should be skip")

	normal := &tokenbucket.RateLimitSettings{Limit: 100, Period: time.Minute}
	require.False(t, normal.IsSkip(), "normal settings should not be skip")
}

func TestRateLimitSettings_LimitInfo(t *testing.T) {
	t.Run("normal", func(t *testing.T) {
		s := &tokenbucket.RateLimitSettings{Limit: 100, Period: time.Minute}
		info := s.LimitInfo()
		require.Equal(t, int64(100), info.Limit)
		require.Equal(t, int64(100), info.Remaining)
		require.Equal(t, int64(60), info.Reset)
	})

	t.Run("unlimited", func(t *testing.T) {
		info := tokenbucket.RateLimitUnlimited.LimitInfo()
		require.Equal(t, int64(math.MaxInt64), info.Limit)
		require.Equal(t, int64(math.MaxInt64), info.Remaining)
	})
}

func TestRateLimitRule_Validate(t *testing.T) {
	tests := []struct {
		name    string
		rule    tokenbucket.RateLimitRule
		wantErr bool
	}{
		{
			"valid IP",
			tokenbucket.RateLimitRule{
				Target:            "192.168.1.1",
				RateLimitSettings: &tokenbucket.RateLimitSettings{Limit: 100, Period: time.Minute},
			},
			false,
		},
		{
			"valid CIDR",
			tokenbucket.RateLimitRule{
				Target:            "10.0.0.0/8",
				RateLimitSettings: &tokenbucket.RateLimitSettings{Limit: 100, Period: time.Minute},
			},
			false,
		},
		{
			"empty target",
			tokenbucket.RateLimitRule{
				Target:            "",
				RateLimitSettings: &tokenbucket.RateLimitSettings{Limit: 100, Period: time.Minute},
			},
			true,
		},
		{
			"invalid target",
			tokenbucket.RateLimitRule{
				Target:            "not-an-ip",
				RateLimitSettings: &tokenbucket.RateLimitSettings{Limit: 100, Period: time.Minute},
			},
			true,
		},
		{
			"nil settings",
			tokenbucket.RateLimitRule{
				Target:            "192.168.1.1",
				RateLimitSettings: nil,
			},
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.rule.Validate()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestRateLimitConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  tokenbucket.RateLimitConfig
		wantErr bool
	}{
		{
			"valid",
			tokenbucket.RateLimitConfig{
				Default: tokenbucket.RateLimitSettings{Limit: 100, Period: time.Minute},
				Rules: []*tokenbucket.RateLimitRule{
					{Target: "192.168.1.1", RateLimitSettings: &tokenbucket.RateLimitSettings{Limit: 200, Period: time.Minute}},
				},
			},
			false,
		},
		{
			"invalid default",
			tokenbucket.RateLimitConfig{
				Default: tokenbucket.RateLimitSettings{Limit: 0, Period: time.Minute},
			},
			true,
		},
		{
			"invalid rule",
			tokenbucket.RateLimitConfig{
				Default: tokenbucket.RateLimitSettings{Limit: 100, Period: time.Minute},
				Rules: []*tokenbucket.RateLimitRule{
					{Target: "not-valid", RateLimitSettings: &tokenbucket.RateLimitSettings{Limit: 100, Period: time.Minute}},
				},
			},
			true,
		},
		{
			"nil rule",
			tokenbucket.RateLimitConfig{
				Default: tokenbucket.RateLimitSettings{Limit: 100, Period: time.Minute},
				Rules:   []*tokenbucket.RateLimitRule{nil},
			},
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
