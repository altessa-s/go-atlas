// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tokenbucket_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/data/limiters/storages/memory"
	"github.com/altessa-s/go-atlas/data/limiters/tokenbucket"
)

func validConfig() *tokenbucket.RateLimitConfig {
	return &tokenbucket.RateLimitConfig{
		Default: tokenbucket.RateLimitSettings{Limit: 10, Period: time.Minute},
	}
}

// mustNew is a test helper that calls New and fails the test on error.
func mustNew(t *testing.T, config *tokenbucket.RateLimitConfig, storage *memory.Provider, opts ...tokenbucket.Option) *tokenbucket.RuleLimiter {
	t.Helper()
	l, err := tokenbucket.New(config, storage, opts...)
	if err != nil {
		t.Fatalf("tokenbucket.New() unexpected error: %v", err)
	}
	return l
}

func TestNew_PanicsOnNilConfig(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("New(nil, ...) should panic")
		}
	}()
	tokenbucket.New(nil, memory.New()) //nolint:errcheck
}

func TestNew_ErrorOnInvalidConfig(t *testing.T) {
	_, err := tokenbucket.New(&tokenbucket.RateLimitConfig{}, memory.New())
	if err == nil {
		t.Error("New(invalid, ...) should return error")
	}
}

func TestNew_ValidConfig(t *testing.T) {
	l, err := tokenbucket.New(validConfig(), memory.New())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if l == nil {
		t.Fatal("New() returned nil")
	}
}

func TestLimit_IPBased_DefaultRule(t *testing.T) {
	cfg := validConfig()
	l := mustNew(t, cfg, memory.New(),
		tokenbucket.WithExtractClientIPAddress(func(_ context.Context) string {
			return "1.2.3.4"
		}),
	)

	info, err := l.Limit(t.Context())
	if err != nil {
		t.Fatalf("Limit() error = %v", err)
	}
	if info == nil {
		t.Fatal("Limit() returned nil info")
	}
	if info.Limit != 10 {
		t.Errorf("Limit = %d, want 10", info.Limit)
	}
}

func TestLimit_IPBased_ExactIPRule(t *testing.T) {
	cfg := &tokenbucket.RateLimitConfig{
		Default: tokenbucket.RateLimitSettings{Limit: 10, Period: time.Minute},
		Rules: []*tokenbucket.RateLimitRule{
			{Target: "1.2.3.4", RateLimitSettings: &tokenbucket.RateLimitSettings{Limit: 50, Period: time.Minute}},
		},
	}
	l := mustNew(t, cfg, memory.New(),
		tokenbucket.WithExtractClientIPAddress(func(_ context.Context) string {
			return "1.2.3.4"
		}),
	)

	info, err := l.Limit(t.Context())
	if err != nil {
		t.Fatalf("Limit() error = %v", err)
	}
	if info.Limit != 50 {
		t.Errorf("Limit = %d, want 50", info.Limit)
	}
}

func TestLimit_IPBased_CIDRRule(t *testing.T) {
	cfg := &tokenbucket.RateLimitConfig{
		Default: tokenbucket.RateLimitSettings{Limit: 10, Period: time.Minute},
		Rules: []*tokenbucket.RateLimitRule{
			{Target: "10.0.0.0/8", RateLimitSettings: &tokenbucket.RateLimitSettings{Limit: 100, Period: time.Minute}},
		},
	}
	l := mustNew(t, cfg, memory.New(),
		tokenbucket.WithExtractClientIPAddress(func(_ context.Context) string {
			return "10.1.2.3"
		}),
	)

	info, err := l.Limit(t.Context())
	if err != nil {
		t.Fatalf("Limit() error = %v", err)
	}
	if info.Limit != 100 {
		t.Errorf("Limit = %d, want 100", info.Limit)
	}
}

func TestLimit_IPBased_NoIP(t *testing.T) {
	l := mustNew(t, validConfig(), memory.New())

	_, err := l.Limit(t.Context())
	if !errors.Is(err, tokenbucket.ErrNoIPFoundOrInvalid) {
		t.Errorf("Limit() error = %v, want ErrNoIPFoundOrInvalid", err)
	}
}

func TestLimit_TokenBased(t *testing.T) {
	cfg := validConfig()
	l := mustNew(t, cfg, memory.New(),
		tokenbucket.WithExtractToken(func(_ context.Context) string {
			return "test-token"
		}),
		tokenbucket.WithClientService(tokenbucket.ClientServiceFunc(
			func(_ context.Context, token string) (*tokenbucket.RateLimitSettings, error) {
				return &tokenbucket.RateLimitSettings{Limit: 200, Period: time.Hour}, nil
			},
		)),
		tokenbucket.WithExtractClientIPAddress(func(_ context.Context) string {
			return "1.2.3.4"
		}),
	)

	info, err := l.Limit(t.Context())
	if err != nil {
		t.Fatalf("Limit() error = %v", err)
	}
	if info.Limit != 200 {
		t.Errorf("Limit = %d, want 200", info.Limit)
	}
}

func TestLimit_TokenBased_FallbackToIP(t *testing.T) {
	cfg := validConfig()
	l := mustNew(t, cfg, memory.New(),
		tokenbucket.WithExtractToken(func(_ context.Context) string {
			return "test-token"
		}),
		tokenbucket.WithClientService(tokenbucket.ClientServiceFunc(
			func(_ context.Context, token string) (*tokenbucket.RateLimitSettings, error) {
				return tokenbucket.RateLimitSkip, nil
			},
		)),
		tokenbucket.WithExtractClientIPAddress(func(_ context.Context) string {
			return "1.2.3.4"
		}),
	)

	info, err := l.Limit(t.Context())
	if err != nil {
		t.Fatalf("Limit() error = %v", err)
	}
	if info.Limit != 10 {
		t.Errorf("Limit = %d, want 10 (default/IP fallback)", info.Limit)
	}
}

func TestLimit_Exceeded(t *testing.T) {
	cfg := &tokenbucket.RateLimitConfig{
		Default: tokenbucket.RateLimitSettings{Limit: 1, Period: time.Minute},
	}
	l := mustNew(t, cfg, memory.New(),
		tokenbucket.WithExtractClientIPAddress(func(_ context.Context) string {
			return "1.2.3.4"
		}),
	)

	// First request should succeed
	if _, err := l.Limit(t.Context()); err != nil {
		t.Fatalf("first Limit() error = %v", err)
	}

	// Second request should exceed
	_, err := l.Limit(t.Context())
	if !errors.Is(err, tokenbucket.ErrLimitExceeded) {
		t.Errorf("second Limit() error = %v, want ErrLimitExceeded", err)
	}
}

func TestLimit_TokenBased_Unlimited(t *testing.T) {
	cfg := validConfig()
	l := mustNew(t, cfg, memory.New(),
		tokenbucket.WithExtractToken(func(_ context.Context) string {
			return "vip-token"
		}),
		tokenbucket.WithClientService(tokenbucket.ClientServiceFunc(
			func(_ context.Context, token string) (*tokenbucket.RateLimitSettings, error) {
				return tokenbucket.RateLimitUnlimited, nil
			},
		)),
		tokenbucket.WithExtractClientIPAddress(func(_ context.Context) string {
			return "1.2.3.4"
		}),
	)

	info, err := l.Limit(t.Context())
	if err != nil {
		t.Fatalf("Limit() error = %v", err)
	}
	if !tokenbucket.RateLimitUnlimited.IsUnlimited() {
		t.Fatal("sanity check: RateLimitUnlimited should be unlimited")
	}
	if info.Remaining != info.Limit {
		t.Errorf("unlimited: Remaining = %d, want = %d", info.Remaining, info.Limit)
	}
}

func TestLimit_TokenBased_ClientServiceError_FallbackToIP(t *testing.T) {
	cfg := validConfig()
	l := mustNew(t, cfg, memory.New(),
		tokenbucket.WithExtractToken(func(_ context.Context) string {
			return "err-token"
		}),
		tokenbucket.WithClientService(tokenbucket.ClientServiceFunc(
			func(_ context.Context, token string) (*tokenbucket.RateLimitSettings, error) {
				return nil, errors.New("service unavailable")
			},
		)),
		tokenbucket.WithExtractClientIPAddress(func(_ context.Context) string {
			return "1.2.3.4"
		}),
	)

	info, err := l.Limit(t.Context())
	if err != nil {
		t.Fatalf("Limit() error = %v", err)
	}
	// Should fall back to default IP-based limit
	if info.Limit != 10 {
		t.Errorf("Limit = %d, want 10 (IP fallback after client service error)", info.Limit)
	}
}

func TestLimit_TokenBased_NoClientService(t *testing.T) {
	cfg := validConfig()
	l := mustNew(t, cfg, memory.New(),
		tokenbucket.WithExtractToken(func(_ context.Context) string {
			return "some-token"
		}),
		// No WithClientService — handleRequest should fall through to IP
		tokenbucket.WithExtractClientIPAddress(func(_ context.Context) string {
			return "1.2.3.4"
		}),
	)

	info, err := l.Limit(t.Context())
	if err != nil {
		t.Fatalf("Limit() error = %v", err)
	}
	if info.Limit != 10 {
		t.Errorf("Limit = %d, want 10 (IP fallback, no client service)", info.Limit)
	}
}

func TestLimit_IPBased_InvalidIP(t *testing.T) {
	l := mustNew(t, validConfig(), memory.New(),
		tokenbucket.WithExtractClientIPAddress(func(_ context.Context) string {
			return "not-an-ip"
		}),
	)

	_, err := l.Limit(t.Context())
	if !errors.Is(err, tokenbucket.ErrNoIPFoundOrInvalid) {
		t.Errorf("Limit() error = %v, want ErrNoIPFoundOrInvalid", err)
	}
}

func TestRuleSorting(t *testing.T) {
	cfg := &tokenbucket.RateLimitConfig{
		Default: tokenbucket.RateLimitSettings{Limit: 1, Period: time.Minute},
		Rules: []*tokenbucket.RateLimitRule{
			{Target: "10.0.0.0/8", RateLimitSettings: &tokenbucket.RateLimitSettings{Limit: 100, Period: time.Minute}},
			{Target: "10.0.0.0/24", RateLimitSettings: &tokenbucket.RateLimitSettings{Limit: 200, Period: time.Minute}},
			{Target: "10.0.0.1", RateLimitSettings: &tokenbucket.RateLimitSettings{Limit: 300, Period: time.Minute}},
		},
	}

	l := mustNew(t, cfg, memory.New(),
		tokenbucket.WithExtractClientIPAddress(func(_ context.Context) string {
			return "10.0.0.1"
		}),
	)

	// Exact IP (300) should take precedence over CIDR matches
	info, err := l.Limit(t.Context())
	if err != nil {
		t.Fatalf("Limit() error = %v", err)
	}
	if info.Limit != 300 {
		t.Errorf("Limit = %d, want 300 (exact IP match)", info.Limit)
	}
}
