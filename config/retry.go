// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"time"

	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Default values for Retry configuration.
const (
	defaultRetryMaxAttempts = 3
	defaultRetryBaseDelay   = 500 * time.Millisecond
	defaultRetryMaxDelay    = 10 * time.Second
	defaultRetryMultiplier  = 1.5
)

// Retry configures exponential backoff retry behavior for external service calls.
// Use [DefaultRetry] to obtain a configuration with sensible defaults.
type Retry struct {
	// MaxAttempts is the maximum number of retry attempts after the initial call.
	// 0 means no retries (call once). -1 means unlimited retries.
	MaxAttempts int `yaml:"maxAttempts" default:"3"`

	// BaseDelay is the initial delay before the first retry.
	BaseDelay time.Duration `yaml:"baseDelay" default:"500ms"`

	// MaxDelay is the maximum delay between retries.
	// Delay increases exponentially up to this limit.
	MaxDelay time.Duration `yaml:"maxDelay" default:"10s"`

	// Multiplier is the exponential backoff factor.
	// Delay is multiplied by this factor after each retry.
	Multiplier float64 `yaml:"multiplier" default:"1.5"`

	// Jitter is the maximum fraction of the computed delay to add as
	// randomized jitter (0.0–1.0). 0 means no jitter (deterministic delays).
	Jitter float64 `yaml:"jitter" default:"0"`

	// MaxElapsedTime is the maximum total wall-clock time for all attempts.
	// 0 means no limit.
	MaxElapsedTime time.Duration `yaml:"maxElapsedTime" default:"0"`
}

// DefaultRetry returns a Retry configuration with sensible defaults.
func DefaultRetry() Retry {
	return Retry{
		MaxAttempts: defaultRetryMaxAttempts,
		BaseDelay:   defaultRetryBaseDelay,
		MaxDelay:    defaultRetryMaxDelay,
		Multiplier:  defaultRetryMultiplier,
	}
}

// Validate performs validation of the Retry configuration.
func (r *Retry) Validate() error {
	return ValidateStruct(r,
		validation.Field(&r.MaxAttempts, validation.Min(-1)),
		validation.Field(&r.BaseDelay, validation.Required, validation.Min(time.Millisecond)),
		validation.Field(&r.MaxDelay, validation.Required, validation.Min(time.Millisecond)),
		validation.Field(&r.Multiplier, validation.Required, validation.Min(1.0)),
		validation.Field(&r.Jitter, validation.Min(0.0), validation.Max(1.0)),
	)
}
