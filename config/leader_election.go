// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"time"

	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Default values for LeaderElector configuration.
const (
	defaultLeaderElectorTTL = 10 * time.Second
)

// LeaderElectorProvider defines the type of leader election provider.
type LeaderElectorProvider string

const (
	// LeaderElectorProviderNats represents the NATS leader election provider.
	LeaderElectorProviderNats LeaderElectorProvider = "nats"
)

// LeaderElector defines the configuration for distributed leader election.
// Used to coordinate leadership among multiple service instances.
//
// Example:
//
//	le := &config.LeaderElector{
//		Provider: config.LeaderElectorProviderNats,
//		Ttl:      10 * time.Second,
//	}
type LeaderElector struct {
	// Provider specifies which leader election provider to use
	Provider LeaderElectorProvider `yaml:"provider" default:"nats"`
	// Ttl defines the time-to-live for leader election locks
	Ttl time.Duration `yaml:"ttl" default:"10s"`
}

// DefaultLeaderElector returns a LeaderElector configuration with default values.
// Note: Provider is left as zero value since it is a required field.
func DefaultLeaderElector() LeaderElector {
	return LeaderElector{
		Ttl: defaultLeaderElectorTTL,
	}
}

// Validate performs validation of the LeaderElector configuration.
// Returns an error if validation fails, nil otherwise.
func (le *LeaderElector) Validate() error {
	return ValidateStruct(le,
		validation.Field(&le.Provider, validation.Required, ozzo_rules.OneOf(LeaderElectorProviderNats)),
		validation.Field(&le.Ttl, ozzo_rules.Duration(), validation.Min(time.Second)),
	)
}
