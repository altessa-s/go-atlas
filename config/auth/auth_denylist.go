// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package authconfig

import (
	"errors"
	"time"

	probfilterconfig "github.com/altessa-s/go-atlas/config/probfilter"
	validationconfig "github.com/altessa-s/go-atlas/config/validation"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// errDenylistTwoRebuildSchedules rejects a rebuild_interval combined with a
// Bloom filter whose rebuildCron is not explicitly empty.
var errDenylistTwoRebuildSchedules = errors.New("rebuild_interval schedules caller-driven rebuilds; " +
	"set filter.bloom.rebuildCron to \"\" so the negcache factory does not schedule a second one " +
	"(an omitted rebuildCron inherits the defaults' cron)")

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

	// RebuildInterval is how often the caller rebuilds the negative filter
	// from the authoritative store through negcache.Cache.Rebuild. Zero (the
	// default) leaves periodic rebuilds to the negcache factory, which
	// schedules them from Filter.Bloom.RebuildCron (or the defaults' cron).
	// With a Bloom filter a non-zero interval requires an explicitly empty
	// Filter.Bloom.RebuildCron, so one filter never gets two schedules.
	// Optional.
	RebuildInterval time.Duration `yaml:"rebuild_interval,omitempty"`

	// MetricsSubsystem overrides the Prometheus metrics subsystem for the
	// negative cache. Empty falls back to the package default. Optional.
	MetricsSubsystem string `yaml:"metrics_subsystem" default:""`

	// Filter is the negative-filter configuration passed to
	// auth/denylist/negcache/factory.NewBuilder.
	Filter probfilterconfig.Filter `yaml:"filter"`
}

// Validate validates the denylist configuration. When Enabled is false it
// returns nil. When enabled it requires KeyPrefix, a non-negative
// RebuildInterval (with a Bloom filter, a positive one only together with an
// explicitly empty Filter.Bloom.RebuildCron), and a valid Filter.
func (c *Denylist) Validate() error {
	return validationconfig.ValidateStructIfEnabled(c.Enabled, c,
		validation.Field(&c.KeyPrefix, validation.Required),
		validation.Field(&c.RebuildInterval, validation.Min(time.Duration(0)), validation.By(func(any) error {
			if c.RebuildInterval <= 0 || c.Filter.Type != probfilterconfig.TypeBloom || c.Filter.Bloom == nil {
				return nil
			}
			if cron := c.Filter.Bloom.RebuildCron; cron == nil || *cron != "" {
				return errDenylistTwoRebuildSchedules
			}
			return nil
		})),
		// Filter.Validate has a pointer receiver, so ozzo's nested-struct
		// check skips the value field — invoke it explicitly.
		validation.Field(&c.Filter, validation.By(func(any) error { return c.Filter.Validate() })),
	)
}

// DefaultDenylist returns a shape-only Denylist configuration: disabled, with
// the default key prefix and a default (bloom) filter shape. Like Default,
// it is a starting point — Validate fails once Enabled is set until real filter
// values are supplied.
func DefaultDenylist() Denylist {
	return Denylist{
		Enabled:   false,
		KeyPrefix: DefaultDenylistKeyPrefix,
		Filter:    probfilterconfig.Filter{Type: probfilterconfig.TypeBloom},
	}
}

// IsEnabled reports whether the denylist is enabled.
func (c *Denylist) IsEnabled() bool {
	return c != nil && c.Enabled
}
