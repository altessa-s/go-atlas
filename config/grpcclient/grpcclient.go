// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package grpcclientconfig

import (
	"time"

	validationconfig "github.com/altessa-s/go-nomos/validation"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Default values. They equal the defaults of transport/grpc/client and its
// dnsresolver package, which a zero or omitted field keeps. An unset jitter
// keeps its default: 0.1 for the refresh jitter, 0.2 for the backoffs.
const (
	defaultResolverRefreshInterval   = 30 * time.Second
	defaultResolverMinInterval       = 2 * time.Second
	defaultResolverLookupTimeout     = 10 * time.Second
	defaultResolverBackoffBaseDelay  = time.Second
	defaultResolverBackoffMultiplier = 1.6
	defaultResolverBackoffMaxDelay   = 5 * time.Second
	defaultConnectBackoffBaseDelay   = time.Second
	defaultConnectBackoffMultiplier  = 1.6
	defaultConnectBackoffMaxDelay    = 2 * time.Minute

	// minRefreshInterval keeps polling from flooding the DNS server.
	minRefreshInterval = time.Second

	// maxRefreshJitter keeps a refresh at least half the refresh interval
	// after the previous one.
	maxRefreshJitter = 0.5
)

// Config holds the name resolution and connection backoff settings of an
// outbound gRPC client (transport/grpc/client). A zero field keeps the
// client's default, so a section left out of the file changes nothing.
type Config struct {
	// Resolver tunes the DNS resolver the client installs for targets without
	// a scheme and dns targets.
	Resolver Resolver `yaml:"resolver"`

	// ConnectBackoff spaces the attempts to connect to one address.
	ConnectBackoff ConnectBackoff `yaml:"connectBackoff"`
}

// Resolver configures the DNS resolver of a gRPC client: periodic
// re-resolution, the rate limit of re-resolution requests, the lookup timeout
// and the backoff after a failed or empty lookup.
type Resolver struct {
	// Builtin selects grpc-go's built-in dns resolver instead; the other
	// resolver settings are then ignored.
	Builtin bool `yaml:"builtin" default:"false"`

	// RefreshInterval is how often the host is resolved again while lookups
	// succeed, so added and removed addresses are seen. Minimum 1s.
	RefreshInterval time.Duration `yaml:"refreshInterval" default:"30s"`

	// RefreshJitter is the largest fraction (0-0.5) by which a refresh
	// interval is shortened at random, so that clients started together do
	// not query the DNS server in lockstep; 0 disables jitter. Unset, it is
	// 0.1.
	RefreshJitter *float64 `yaml:"refreshJitter"`

	// MinInterval is the shortest pause between the end of a lookup and a
	// lookup that gRPC requests after a connection failure. It may not
	// exceed RefreshInterval.
	MinInterval time.Duration `yaml:"minInterval" default:"2s"`

	// LookupTimeout bounds one lookup.
	LookupTimeout time.Duration `yaml:"lookupTimeout" default:"10s"`

	// Backoff spaces the lookups after a failed or empty one.
	Backoff ResolverBackoff `yaml:"backoff"`
}

// ResolverBackoff is the exponential backoff between failed lookups. Its cap
// bounds how long a client takes to see addresses that return after an
// outage.
type ResolverBackoff struct {
	// BaseDelay is the delay after the first failed lookup.
	BaseDelay time.Duration `yaml:"baseDelay" default:"1s"`

	// Multiplier is the factor the delay grows by after each further failed
	// lookup. Minimum 1.
	Multiplier float64 `yaml:"multiplier" default:"1.6"`

	// Jitter is the fraction (0-1) by which each delay is randomized; 0
	// disables jitter. Unset, it is 0.2.
	Jitter *float64 `yaml:"jitter"`

	// MaxDelay caps the delay between failed lookups. It may not be below
	// BaseDelay.
	MaxDelay time.Duration `yaml:"maxDelay" default:"5s"`
}

// ConnectBackoff is the exponential backoff between attempts to connect to
// one address. With a target that always resolves (a ClusterIP service, an
// IP address) it alone decides how fast a client reconnects after an outage.
type ConnectBackoff struct {
	// BaseDelay is the delay after the first failed connection attempt.
	BaseDelay time.Duration `yaml:"baseDelay" default:"1s"`

	// Multiplier is the factor the delay grows by after each further failed
	// attempt. Minimum 1.
	Multiplier float64 `yaml:"multiplier" default:"1.6"`

	// Jitter is the fraction (0-1) by which each delay is randomized; 0
	// disables jitter. Unset, it is 0.2.
	Jitter *float64 `yaml:"jitter"`

	// MaxDelay caps the delay between connection attempts. It may not be
	// below BaseDelay.
	MaxDelay time.Duration `yaml:"maxDelay" default:"2m"`
}

// Default returns a Config with the client defaults; the jitters are left
// unset, which keeps their default.
func Default() Config {
	return Config{Resolver: DefaultResolver(), ConnectBackoff: DefaultConnectBackoff()}
}

// DefaultResolver returns a Resolver with the client defaults.
func DefaultResolver() Resolver {
	return Resolver{
		RefreshInterval: defaultResolverRefreshInterval,
		MinInterval:     defaultResolverMinInterval,
		LookupTimeout:   defaultResolverLookupTimeout,
		Backoff: ResolverBackoff{
			BaseDelay:  defaultResolverBackoffBaseDelay,
			Multiplier: defaultResolverBackoffMultiplier,
			MaxDelay:   defaultResolverBackoffMaxDelay,
		},
	}
}

// DefaultConnectBackoff returns a ConnectBackoff with the client defaults,
// those of grpc-go.
func DefaultConnectBackoff() ConnectBackoff {
	return ConnectBackoff{
		BaseDelay:  defaultConnectBackoffBaseDelay,
		Multiplier: defaultConnectBackoffMultiplier,
		MaxDelay:   defaultConnectBackoffMaxDelay,
	}
}

// Validate performs validation of the Config configuration.
func (c *Config) Validate() error {
	return validationconfig.ValidateStruct(c,
		validationconfig.NestedField(&c.Resolver),
		validationconfig.NestedField(&c.ConnectBackoff),
	)
}

// Validate performs validation of the Resolver configuration. Zero
// durations stand for their defaults; negative ones are rejected.
func (r *Resolver) Validate() error {
	refresh := r.RefreshInterval
	if refresh == 0 {
		refresh = defaultResolverRefreshInterval
	}
	return validationconfig.ValidateStruct(r,
		validation.Field(&r.RefreshInterval, validation.Min(minRefreshInterval)),
		validation.Field(&r.RefreshJitter, validation.Min(0.0), validation.Max(maxRefreshJitter)),
		validation.Field(&r.MinInterval, validation.Min(time.Duration(0)), validation.Max(refresh)),
		validation.Field(&r.LookupTimeout, validation.Min(time.Duration(0))),
		validationconfig.NestedField(&r.Backoff),
	)
}

// Validate performs validation of the ResolverBackoff configuration.
func (b *ResolverBackoff) Validate() error {
	return validationconfig.ValidateStruct(b, backoffRules(&b.BaseDelay, &b.Multiplier, &b.Jitter, &b.MaxDelay, defaultResolverBackoffMaxDelay)...)
}

// Validate performs validation of the ConnectBackoff configuration.
func (b *ConnectBackoff) Validate() error {
	return validationconfig.ValidateStruct(b, backoffRules(&b.BaseDelay, &b.Multiplier, &b.Jitter, &b.MaxDelay, defaultConnectBackoffMaxDelay)...)
}

// backoffRules validates the fields of a backoff: non-negative delays, a
// base delay no greater than the (default) cap, a multiplier of at least 1
// and a jitter between 0 and 1.
func backoffRules(baseDelay *time.Duration, multiplier *float64, jitter **float64, maxDelay *time.Duration,
	defaultMaxDelay time.Duration,
) []*validation.FieldRules {
	ceiling := *maxDelay
	if ceiling == 0 {
		ceiling = defaultMaxDelay
	}
	return []*validation.FieldRules{
		validation.Field(baseDelay, validation.Min(time.Duration(0)), validation.Max(ceiling)),
		validation.Field(multiplier, validation.Min(1.0)),
		validation.Field(jitter, validation.Min(0.0), validation.Max(1.0)),
		validation.Field(maxDelay, validation.Min(time.Duration(0))),
	}
}
