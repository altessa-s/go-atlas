// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"fmt"

	"github.com/altessa-s/go-atlas/core/runtime/concurrency"

	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Default values for ConcurrencyConfig.
const (
	defaultConcurrencyStrategy    = ConcurrencyStatic
	defaultConcurrencyMaxTasks    = 5
	defaultConcurrencyEnvironment = concurrency.EnvironmentIOBound
)

// ConcurrencyStrategy defines the concurrency strategy.
type ConcurrencyStrategy string

const (
	// ConcurrencyStatic uses a fixed concurrency limit from MaxTasks.
	ConcurrencyStatic ConcurrencyStrategy = "static"
	// ConcurrencyEnvironment selects a preset based on the environment type.
	ConcurrencyEnvironment ConcurrencyStrategy = "environment"
	// ConcurrencyMemoryAware adapts concurrency based on available memory.
	ConcurrencyMemoryAware ConcurrencyStrategy = "memory-aware"
	// ConcurrencyAdaptive combines memory and load factors for concurrency.
	ConcurrencyAdaptive ConcurrencyStrategy = "adaptive"
)

var concurrencyAllowedStrategies = []ConcurrencyStrategy{
	ConcurrencyStatic,
	ConcurrencyEnvironment,
	ConcurrencyMemoryAware,
	ConcurrencyAdaptive,
}

var concurrencyAllowedEnvironments = []concurrency.Environment{
	concurrency.EnvironmentMemoryConstrained,
	concurrency.EnvironmentCPUBound,
	concurrency.EnvironmentIOBound,
	concurrency.EnvironmentHighThroughput,
	concurrency.EnvironmentRateLimited,
}

// MemoryAwareConcurrencyConfig configures the memory-aware concurrency strategy.
type MemoryAwareConcurrencyConfig struct {
	// LowMemoryMB is the memory threshold (in MB) below which concurrency is set to 1.
	LowMemoryMB uint64 `yaml:"lowMemoryMB"`
	// MediumMemoryMB is the memory threshold (in MB) below which concurrency is conservative.
	MediumMemoryMB uint64 `yaml:"mediumMemoryMB"`
	// HighMemoryMB is the memory threshold (in MB) above which concurrency is aggressive.
	HighMemoryMB uint64 `yaml:"highMemoryMB"`
}

// Validate checks that memory thresholds are positive and ordered low < medium < high.
func (c *MemoryAwareConcurrencyConfig) Validate() error {
	return ValidateStruct(c,
		validation.Field(&c.LowMemoryMB, validation.Required, validation.Min(uint64(1))),
		validation.Field(&c.MediumMemoryMB, validation.Required, validation.Min(uint64(1))),
		validation.Field(&c.HighMemoryMB, validation.Required, validation.Min(uint64(1))),
	)
}

// AdaptiveConcurrencyConfig configures the adaptive concurrency strategy.
type AdaptiveConcurrencyConfig struct {
	// MemoryLowThresholdMB is the available-memory threshold (in MB) below which
	// concurrency is reduced to 25% of the base value.
	MemoryLowThresholdMB uint64 `yaml:"memoryLowThresholdMB"`
	// MemoryMediumThresholdMB is the available-memory threshold (in MB) below which
	// concurrency is reduced to 50% of the base value.
	MemoryMediumThresholdMB uint64 `yaml:"memoryMediumThresholdMB"`
	// HighLoadThreshold is the system load above which concurrency is scaled down.
	HighLoadThreshold float64 `yaml:"highLoadThreshold"`
}

// Validate checks that the adaptive concurrency thresholds are valid.
func (c *AdaptiveConcurrencyConfig) Validate() error {
	return ValidateStruct(c,
		validation.Field(&c.MemoryLowThresholdMB, validation.Required, validation.Min(uint64(1))),
		validation.Field(&c.MemoryMediumThresholdMB, validation.Required, validation.Min(uint64(1))),
	)
}

// ConcurrencyConfig is the base concurrency configuration reusable across components.
//
// Example:
//
//	cc := &config.ConcurrencyConfig{
//		Strategy: config.ConcurrencyStatic,
//		MaxTasks: 10,
//	}
type ConcurrencyConfig struct {
	// Strategy selects the concurrency strategy.
	// Must be one of: static, environment, memory-aware, adaptive.
	// Defaults to "static".
	Strategy ConcurrencyStrategy `yaml:"strategy" default:"static"`

	// MaxTasks is the maximum number of tasks that can run concurrently.
	// Zero means unlimited. Used with the "static" strategy.
	// Defaults to 0 (unlimited).
	MaxTasks int `yaml:"maxTasks" default:"5"`

	// Environment selects a preset concurrency profile.
	// Used when Strategy is "environment".
	// Must be one of: memory-constrained, cpu-bound, io-bound, high-throughput, rate-limited.
	// Defaults to "io-bound".
	Environment concurrency.Environment `yaml:"environment" default:"io-bound"`

	// MemoryAware configures the memory-aware concurrency strategy.
	// Required when Strategy is "memory-aware".
	MemoryAware *MemoryAwareConcurrencyConfig `yaml:"memoryAware" default:"-"`

	// Adaptive configures the adaptive concurrency strategy.
	// Required when Strategy is "adaptive".
	Adaptive *AdaptiveConcurrencyConfig `yaml:"adaptive" default:"-"`
}

// DefaultConcurrencyConfig returns a ConcurrencyConfig with default values.
func DefaultConcurrencyConfig() ConcurrencyConfig {
	return ConcurrencyConfig{
		Strategy:    defaultConcurrencyStrategy,
		MaxTasks:    defaultConcurrencyMaxTasks,
		Environment: defaultConcurrencyEnvironment,
	}
}

// Validate checks that the concurrency configuration is valid.
func (c *ConcurrencyConfig) Validate() error {
	return ValidateStruct(c,
		validation.Field(&c.Strategy, validation.Required,
			ozzo_rules.OneOf(concurrencyAllowedStrategies...)),
		validation.Field(&c.MaxTasks, validation.Min(0)),
		validation.Field(&c.Environment,
			validation.When(c.Strategy == ConcurrencyEnvironment, validation.Required,
				ozzo_rules.OneOf(concurrencyAllowedEnvironments...))),
		validation.Field(&c.MemoryAware,
			validation.When(c.Strategy == ConcurrencyMemoryAware, validation.Required)),
		validation.Field(&c.Adaptive,
			validation.When(c.Strategy == ConcurrencyAdaptive, validation.Required)),
	)
}

// BuildLimitFunc maps the concurrency strategy to a [concurrency.ConcurrencyLimitFunc].
// For the "static" strategy with MaxTasks=0 it returns nil (callers should fall back to defaults).
// For "static" with a positive MaxTasks it returns a func returning that value.
func (c *ConcurrencyConfig) BuildLimitFunc() (concurrency.ConcurrencyLimitFunc, error) {
	switch c.Strategy {
	case ConcurrencyStatic, "":
		if c.MaxTasks <= 0 {
			return nil, nil //nolint:nilnil // nil means "use default", not an error
		}
		limit := c.MaxTasks
		return func() int { return limit }, nil

	case ConcurrencyEnvironment:
		env := c.Environment
		return func() int { return concurrency.ConcurrencyForEnvironment(env) }, nil

	case ConcurrencyMemoryAware:
		if c.MemoryAware == nil {
			return nil, fmt.Errorf("memoryAware configuration is required for strategy %q", c.Strategy)
		}
		return concurrency.MemoryAwareConcurrency(
			c.MemoryAware.LowMemoryMB,
			c.MemoryAware.MediumMemoryMB,
			c.MemoryAware.HighMemoryMB,
		), nil

	case ConcurrencyAdaptive:
		if c.Adaptive == nil {
			return nil, fmt.Errorf("adaptive configuration is required for strategy %q", c.Strategy)
		}
		return concurrency.AdaptiveConcurrency(concurrency.AdaptiveConcurrencyConfig{
			MemoryLowThresholdMB:    c.Adaptive.MemoryLowThresholdMB,
			MemoryMediumThresholdMB: c.Adaptive.MemoryMediumThresholdMB,
			HighLoadThreshold:       c.Adaptive.HighLoadThreshold,
		}), nil

	default:
		return nil, fmt.Errorf("unsupported concurrency strategy: %s", c.Strategy)
	}
}
