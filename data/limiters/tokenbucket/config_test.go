// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tokenbucket_test

import (
	"math"
	"testing"
	"time"

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
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRateLimitSettings_IsUnlimited(t *testing.T) {
	if !tokenbucket.RateLimitUnlimited.IsUnlimited() {
		t.Error("RateLimitUnlimited.IsUnlimited() = false, want true")
	}

	normal := &tokenbucket.RateLimitSettings{Limit: 100, Period: time.Minute}
	if normal.IsUnlimited() {
		t.Error("normal settings should not be unlimited")
	}
}

func TestRateLimitSettings_IsSkip(t *testing.T) {
	if !tokenbucket.RateLimitSkip.IsSkip() {
		t.Error("RateLimitSkip.IsSkip() = false, want true")
	}

	var nilSettings *tokenbucket.RateLimitSettings
	if !nilSettings.IsSkip() {
		t.Error("nil settings should be skip")
	}

	normal := &tokenbucket.RateLimitSettings{Limit: 100, Period: time.Minute}
	if normal.IsSkip() {
		t.Error("normal settings should not be skip")
	}
}

func TestRateLimitSettings_LimitInfo(t *testing.T) {
	t.Run("normal", func(t *testing.T) {
		s := &tokenbucket.RateLimitSettings{Limit: 100, Period: time.Minute}
		info := s.LimitInfo()
		if info.Limit != 100 {
			t.Errorf("Limit = %d, want 100", info.Limit)
		}
		if info.Remaining != 100 {
			t.Errorf("Remaining = %d, want 100", info.Remaining)
		}
		if info.Reset != 60 {
			t.Errorf("Reset = %d, want 60", info.Reset)
		}
	})

	t.Run("unlimited", func(t *testing.T) {
		info := tokenbucket.RateLimitUnlimited.LimitInfo()
		if info.Limit != math.MaxInt64 {
			t.Errorf("Limit = %d, want MaxInt64", info.Limit)
		}
		if info.Remaining != math.MaxInt64 {
			t.Errorf("Remaining = %d, want MaxInt64", info.Remaining)
		}
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
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
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
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
