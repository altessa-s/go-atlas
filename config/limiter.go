// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"time"

	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Default values for Limiter configuration.
const (
	defaultLimiterIpCacheSize       = 1000
	defaultLimiterDefaultRuleLimit  = int64(1000)
	defaultLimiterDefaultRulePeriod = time.Hour
)

// LimiterDefaultRule defines the default rate limiting settings.
// Applied when no specific target rule matches the client.
type LimiterDefaultRule struct {
	// Limit is the maximum number of requests allowed within the period.
	// Must be a positive integer greater than 0.
	Limit int64 `yaml:"limit" default:"1000"`

	// Period is the time window for the limit (e.g., "1h", "30m", "60s").
	// Must be a valid duration string parseable by time.ParseDuration.
	Period time.Duration `yaml:"period" default:"1h"`
}

// DefaultLimiterDefaultRule returns a LimiterDefaultRule configuration with default values.
func DefaultLimiterDefaultRule() LimiterDefaultRule {
	return LimiterDefaultRule{
		Limit:  defaultLimiterDefaultRuleLimit,
		Period: defaultLimiterDefaultRulePeriod,
	}
}

// Validate performs validation of the default rule configuration.
// Ensures limit is positive and period is a valid duration.
func (d *LimiterDefaultRule) Validate() error {
	return ValidateStruct(d,
		validation.Field(&d.Limit, validation.Required, validation.Min(1)),
		validation.Field(&d.Period, validation.Required, ozzo_rules.Duration(), validation.Min(time.Second)),
	)
}

// LimiterTargetRule defines rate limiting settings for specific targets.
// Targets can be IP addresses, CIDR blocks, or other client identifiers.
type LimiterTargetRule struct {
	// Target specifies the client identifier (IP address, CIDR block, etc.).
	// Examples: "192.168.1.100", "10.0.0.0/8", "client-id-123"
	Target string `yaml:"target"`

	// Limit is the maximum number of requests allowed within the period.
	// Must be a positive integer greater than 0.
	Limit int64 `yaml:"limit"`

	// Period is the time window for the limit (e.g., "1h", "30m", "60s").
	// Must be a valid duration string parseable by time.ParseDuration.
	Period time.Duration `yaml:"period"`
}

// Validate performs validation of the target rule configuration.
// Ensures target is specified, limit is positive, and period is valid.
func (t *LimiterTargetRule) Validate() error {
	return ValidateStruct(t,
		validation.Field(&t.Target, validation.Required),
		validation.Field(&t.Limit, validation.Required, validation.Min(1)),
		validation.Field(&t.Period, validation.Required, ozzo_rules.Duration(), validation.Min(time.Second)),
	)
}

// LimiterRules defines the complete set of rate limiting rules.
// Includes default settings and specific rules for targeted clients.
type LimiterRules struct {
	// Default contains the default rate limiting settings.
	// Applied when no specific target rule matches the client.
	Default *LimiterDefaultRule `yaml:"default"`

	// Targets contains specific rate limiting rules for different clients.
	// Rules are evaluated in order; the first matching rule is applied.
	// If no target rule matches, the default rule (if specified) is used.
	Targets []LimiterTargetRule `yaml:"targets,omitempty"`
}

// Validate performs validation of the rules configuration.
// Ensures default rule and all target rules are properly configured.
func (r *LimiterRules) Validate() error {
	return ValidateStruct(r,
		validation.Field(&r.Default, validation.Required),
		validation.Field(&r.Targets, validation.Each(validation.By(func(value any) error {
			if rule, ok := value.(LimiterTargetRule); ok {
				return rule.Validate()
			}
			return nil
		}))),
	)
}

// Limiter defines the configuration for rate limiting.
// It controls rate limiting behavior including storage, rules, and IP-based settings.
//
// Example:
//
//	limiter := &config.Limiter{
//		Storage: &config.CacheStorageConfig{Type: config.CacheStorageTypeMemory},
//		Rules:   &config.LimiterRules{Default: &config.LimiterDefaultRule{Limit: 1000, Period: time.Hour}},
//	}
type Limiter struct {
	// IpCacheSize defines the size of the LRU cache for IP-to-settings mappings.
	// This cache optimizes repeated lookups for the same client IP.
	IpCacheSize int `yaml:"ipCacheSize" default:"1000"`

	// Storage defines the storage configuration for rate limit counters.
	// Required, specifies backend and settings.
	Storage *CacheStorageConfig `yaml:"storage"`

	// Rules contains the rate limiting rules configuration.
	// Includes default settings and specific rules for targeted clients.
	Rules *LimiterRules `yaml:"rules"`
}

// DefaultLimiter returns a Limiter configuration with default values.
// Note: Storage and Rules are left as nil since they are required fields.
func DefaultLimiter() Limiter {
	return Limiter{
		IpCacheSize: defaultLimiterIpCacheSize,
	}
}

// Validate performs validation of the rate limiter configuration.
// Ensures that storage and rules are correctly configured.
// Returns an error if validation fails, nil otherwise.
func (l *Limiter) Validate() error {
	return ValidateStruct(l,
		validation.Field(&l.IpCacheSize, validation.Min(0)),
		validation.Field(&l.Storage, validation.Required),
		validation.Field(&l.Rules, validation.Required),
	)
}
