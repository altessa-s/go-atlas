// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"time"

	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// DefaultDenylistKeyPrefix is the default Redis key prefix for the authoritative
// token-revocation store built by auth/denylist/storages/redis.New.
const DefaultDenylistKeyPrefix = "denylist:revoked:"

// Denylist is the configuration for the token-revocation denylist with a
// negative-cache in front of a distributed authoritative store. It carries only
// the values the caller feeds to the runtime factories:
// auth/denylist/negcache/factory.NewBuilder for the negative filter and
// auth/denylist/storages/redis.New for the authoritative store. The Redis client
// and any callbacks are wired in code.
//
// Example:
//
//	denylist:
//	  enabled: true
//	  key_prefix: "denylist:revoked:"
//	  rebuild_interval: 5m
//	  filter:
//	    type: bloom
//	    bloom:
//	      expectedItems: 1000000
type Denylist struct {
	// Enabled turns the denylist on. When false the whole component is skipped
	// and validation is lenient — the remaining fields are not required.
	Enabled bool `yaml:"enabled" default:"false"`

	// KeyPrefix is the Redis key prefix for the authoritative revocation store
	// (auth/denylist/storages/redis.WithKeyPrefix). Required when enabled.
	// Defaults to "denylist:revoked:".
	KeyPrefix string `yaml:"key_prefix" default:"denylist:revoked:"`

	// RebuildInterval is how often the negative filter is rebuilt from the
	// authoritative store. Zero disables periodic rebuilds. Optional.
	RebuildInterval time.Duration `yaml:"rebuild_interval,omitempty"`

	// MetricsSubsystem overrides the Prometheus metrics subsystem for the
	// negative cache. Empty falls back to the package default. Optional.
	MetricsSubsystem string `yaml:"metrics_subsystem" default:""`

	// Filter is the negative-filter configuration passed to
	// auth/denylist/negcache/factory.NewBuilder.
	Filter ProbabilisticFilterConfig `yaml:"filter"`
}

// Validate validates the denylist configuration. When Enabled is false it
// returns nil. When enabled it requires KeyPrefix, a non-negative
// RebuildInterval, and a valid Filter.
func (c *Denylist) Validate() error {
	return ValidateStructIfEnabled(c.Enabled, c,
		validation.Field(&c.KeyPrefix, validation.Required),
		validation.Field(&c.RebuildInterval, validation.Min(time.Duration(0))),
		// Filter.Validate has a pointer receiver, so ozzo's nested-struct
		// check skips the value field — invoke it explicitly.
		validation.Field(&c.Filter, validation.By(func(any) error { return c.Filter.Validate() })),
	)
}

// DefaultDenylist returns a shape-only Denylist configuration: disabled, with
// the default key prefix and a default (bloom) filter shape. Like DefaultAuth,
// it is a starting point — Validate fails once Enabled is set until real filter
// values are supplied.
func DefaultDenylist() Denylist {
	return Denylist{
		Enabled:   false,
		KeyPrefix: DefaultDenylistKeyPrefix,
		Filter:    ProbabilisticFilterConfig{Type: ProbabilisticFilterTypeBloom},
	}
}

// IsEnabled reports whether the denylist is enabled.
func (c *Denylist) IsEnabled() bool {
	return c != nil && c.Enabled
}
