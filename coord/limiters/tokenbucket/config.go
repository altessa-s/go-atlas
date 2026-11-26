// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tokenbucket

import (
	"errors"
	"fmt"
	"math"
	"net"
	"time"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	limiters "github.com/altessa-s/go-atlas/coord/limiters"
)

// RateLimitSettings defines the basic parameters for a rate limit.
// It specifies how many requests are allowed within a given time period using
// a sliding window algorithm. The combination of Limit and Period determines
// the maximum sustainable request rate.
//
// Examples:
//   - Limit: 100, Period: 1 minute = 100 requests per minute
//   - Limit: 10, Period: 1 second = 10 requests per second
//   - Limit: 1000, Period: 1 hour = 1000 requests per hour
type RateLimitSettings struct {
	// Limit is the maximum number of requests allowed within the time period.
	// Must be greater than 0. Higher values allow more requests.
	Limit int64
	// Period is the time window for which the limit applies.
	// Must be greater than 0. Longer periods allow burst behavior.
	Period time.Duration
}

// IsUnlimited returns true if the rate limit settings indicate no limit.
func (s *RateLimitSettings) IsUnlimited() bool {
	return s != nil && s.Limit == math.MaxInt64 && s.Period == math.MaxInt64
}

// IsSkip returns true if the rate limit settings indicate that rate limiting should be skipped.
func (s *RateLimitSettings) IsSkip() bool {
	return s == nil || s.Limit == math.MinInt64 && s.Period == math.MinInt64
}

// LimitInfo returns a [limiters.LimitInfo] snapshot derived from these settings, with
// Reset as the Unix time one period from now — the same absolute timestamp
// storages report. For unlimited settings, all fields are set to
// [math.MaxInt64].
func (s *RateLimitSettings) LimitInfo() *limiters.LimitInfo {
	if s.IsUnlimited() {
		return &limiters.LimitInfo{
			Limit:     math.MaxInt64,
			Remaining: math.MaxInt64,
			Reset:     math.MaxInt64,
		}
	}

	return &limiters.LimitInfo{
		Limit:     s.Limit,
		Remaining: s.Limit, // Initially, all requests are remaining
		Reset:     time.Now().Add(s.Period).Unix(),
	}
}

// Validate checks that the rate limit settings are valid and can be used
// for rate limiting operations. Both Limit and Period must be positive values.
//
// Returns an error if the settings are invalid, nil otherwise.
func (s *RateLimitSettings) Validate() error {
	if s == nil || s.Limit <= 0 {
		return fmt.Errorf("%w: limit must be greater than 0", coreerrs.ErrInvalidArgument)
	}
	if s.Period <= 0 {
		return fmt.Errorf("%w: period must be greater than 0", coreerrs.ErrInvalidArgument)
	}
	return nil
}

// RateLimitUnlimited is a predefined RateLimitSettings instance that represents no rate limiting.
// It can be used to indicate that a client or IP address has unlimited access.
// Note: Limit is set to MaxInt64 to signify no limit, and Period is set to MaxInt64.
var RateLimitUnlimited = &RateLimitSettings{
	Limit:  math.MaxInt64,
	Period: math.MaxInt64,
}

// RateLimitSkip is a predefined RateLimitSettings instance that effectively disables rate limiting.
// It can be used in scenarios where rate limiting should be bypassed entirely.
// Note: Limit and Period are set to MinInt64 to signify skipping rate limiting.
var RateLimitSkip = &RateLimitSettings{
	Limit:  math.MinInt64,
	Period: math.MinInt64,
}

// RateLimitRule defines the rate limit settings for a specific IP or subnet.
// Rules allow setting custom rate limits for specific network addresses,
// enabling different rate limits for different classes of clients.
//
// Rules are processed in order of specificity during matching:
//  1. Exact IP addresses (e.g., "192.168.1.100")
//  2. CIDR subnets, most specific first (e.g., "/24" before "/16")
//
// Common use cases:
//   - Higher limits for internal networks
//   - Lower limits for external/public networks
//   - Special limits for known problematic IP ranges
//   - Administrative access with unlimited rates
type RateLimitRule struct {
	// Target is the IP address or CIDR subnet to apply this rule to.
	// Must be a valid IPv4 or IPv6 address or CIDR notation.
	// Examples: "192.168.1.1", "10.0.0.0/8", "2001:db8::/32"
	Target string
	// RateLimitSettings contains the rate limit parameters for this target.
	// Must not be nil and must pass validation.
	*RateLimitSettings
}

// Validate checks that the rate limit rule is valid and can be used for matching.
// It verifies that the target is a valid IP address or CIDR notation and that
// the embedded rate limit settings are valid.
//
// Returns an error describing the validation failure, nil if valid.
func (r *RateLimitRule) Validate() error {
	if r.Target == "" {
		return errors.New("target must not be empty")
	}
	if err := validateIPOrCIDR(r.Target); err != nil {
		return err
	}
	if r.RateLimitSettings == nil {
		return errors.New("rate limit settings must not be nil")
	}
	return r.RateLimitSettings.Validate()
}

// validateRules checks every rule passed through [WithRules].
func validateRules(rules []*RateLimitRule) error {
	for i, rule := range rules {
		if rule == nil {
			return fmt.Errorf("rule at index %d is nil", i)
		}
		if err := rule.Validate(); err != nil {
			return coreerrs.Wrapf(err, "invalid rule at index %d", i)
		}
	}
	return nil
}

// validateIPOrCIDR checks if a string is a valid IP address or CIDR notation.
func validateIPOrCIDR(target string) error {
	if net.ParseIP(target) == nil {
		if _, _, err := net.ParseCIDR(target); err != nil {
			return fmt.Errorf("target '%s' is not a valid IP address or CIDR notation", target)
		}
	}
	return nil
}
