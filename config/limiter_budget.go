// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"time"

	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Default values for BudgetLimiter configuration.
const (
	defaultBudgetLimiterPeriod = 24 * time.Hour
)

// BudgetLimiter configures a distributed budget limiter for outbound requests.
// It tracks the total number of requests within a configurable time period
// using a shared storage backend for consistent enforcement across replicas.
type BudgetLimiter struct {
	// Limit is the maximum number of requests allowed within the period.
	// Must be a positive integer greater than 0.
	Limit int64 `yaml:"limit"`

	// Period is the time window for the budget counter.
	// Defaults to 24 hours.
	Period time.Duration `yaml:"period" default:"24h"`

	// Storage defines the storage configuration for budget counters.
	// Required, specifies backend and settings (memory, nats, or redis).
	Storage *CacheStorageConfig `yaml:"storage"`
}

// Validate performs validation of the budget limiter configuration.
// Ensures limit is positive, period is at least one second, and storage is configured.
func (b *BudgetLimiter) Validate() error {
	return ValidateStruct(b,
		validation.Field(&b.Limit, validation.Required, validation.Min(int64(1))),
		validation.Field(&b.Period, validation.Required, ozzo_rules.Duration(), validation.Min(time.Second)),
		validation.Field(&b.Storage, validation.Required),
	)
}

// DefaultBudgetLimiter returns a BudgetLimiter configuration with default values.
// Note: Storage is left as nil since it is a required field.
func DefaultBudgetLimiter() BudgetLimiter {
	return BudgetLimiter{
		Period: defaultBudgetLimiterPeriod,
	}
}
