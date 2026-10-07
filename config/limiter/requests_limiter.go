// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package limiterconfig

import (
	"time"

	validationconfig "github.com/altessa-s/go-atlas/config/validation"
	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Default values for RequestRate configuration.
const (
	defaultRequestsLimiterRate   = 2500
	defaultRequestsLimiterPeriod = 24 * time.Hour
	defaultRequestsLimiterBurst  = 100
)

// RequestRate represents the configuration for rate limiting HTTP requests.
// It implements a token bucket algorithm to control the rate of incoming requests
// and prevent system overload while allowing burst traffic.
type RequestRate struct {
	// Rate is the maximum number of requests allowed per period.
	// Defaults to 2500 requests per period.
	// This represents the sustained request rate over the configured period.
	Rate int `yaml:"rate" default:"2500"`

	// Period is the time window for the rate limit.
	// Defaults to 24 hours.
	// The rate limit resets every period.
	Period time.Duration `yaml:"period" default:"24h"`

	// Burst is the maximum number of requests that can be processed
	// in a short burst above the sustained rate.
	// Defaults to 100 requests.
	// This allows handling traffic spikes while maintaining overall rate limits.
	Burst int `yaml:"burst" default:"100"`
}

// DefaultRequestRate returns a RequestRate configuration with default values.
func DefaultRequestRate() RequestRate {
	return RequestRate{
		Rate:   defaultRequestsLimiterRate,
		Period: defaultRequestsLimiterPeriod,
		Burst:  defaultRequestsLimiterBurst,
	}
}

// Validate performs validation on the RequestRate configuration.
// It ensures that rate, period, and burst values are properly specified
// and within acceptable ranges for rate limiting functionality.
//
// Returns an error if any validation rules fail.
func (l *RequestRate) Validate() error {
	return validationconfig.ValidateStruct(l,
		validation.Field(&l.Rate, validation.Required),
		validation.Field(&l.Period, ozzo_rules.Duration()),
		validation.Field(&l.Burst, validation.Required),
	)
}
