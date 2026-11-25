// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"github.com/altessa-s/go-atlas-core/runtime/concurrency"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	schedulerconfig "github.com/altessa-s/go-atlas/config/scheduler"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
)

// BuildLimitFunc maps the concurrency strategy to a [concurrency.ConcurrencyLimitFunc].
// For the "static" strategy with MaxTasks=0 it returns nil (callers should fall back to defaults).
// For "static" with a positive MaxTasks it returns a func returning that value.
func BuildLimitFunc(c schedulerconfig.Concurrency) (concurrency.ConcurrencyLimitFunc, error) {
	switch c.Strategy {
	case schedulerconfig.ConcurrencyStatic, "":
		if c.MaxTasks <= 0 {
			return nil, nil //nolint:nilnil // nil means "use default", not an error
		}
		limit := c.MaxTasks
		return func() int { return limit }, nil

	case schedulerconfig.ConcurrencyEnvironment:
		env := c.Environment
		return func() int { return concurrency.ConcurrencyForEnvironment(env) }, nil

	case schedulerconfig.ConcurrencyMemoryAware:
		if c.MemoryAware == nil {
			return nil, corefactory.Errorf(coreerrs.ErrInvalidArgument, "memoryAware configuration is required for strategy %q", c.Strategy)
		}
		return concurrency.MemoryAwareConcurrency(
			c.MemoryAware.LowMemoryMB,
			c.MemoryAware.MediumMemoryMB,
			c.MemoryAware.HighMemoryMB,
		), nil

	case schedulerconfig.ConcurrencyAdaptive:
		if c.Adaptive == nil {
			return nil, corefactory.Errorf(coreerrs.ErrInvalidArgument, "adaptive configuration is required for strategy %q", c.Strategy)
		}
		return concurrency.AdaptiveConcurrency(
			c.Adaptive.MemoryLowThresholdMB, c.Adaptive.MemoryMediumThresholdMB, nil, c.Adaptive.HighLoadThreshold,
		), nil

	default:
		return nil, corefactory.Errorf(coreerrs.ErrInvalidArgument, "unsupported concurrency strategy: %s", c.Strategy)
	}
}
