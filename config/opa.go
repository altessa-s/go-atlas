// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"time"

	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

const (
	// DefaultOPAQuery is the default Rego query for authorization decisions.
	DefaultOPAQuery = "data.profiles.authz.allow"

	// DefaultOPACacheTTL is the default cache TTL for authorization decisions.
	DefaultOPACacheTTL = 5 * time.Minute

	// DefaultOPADecisionLogging is the default decision logging setting.
	DefaultOPADecisionLogging = false

	// DefaultOPAWatchBundle is the default bundle watching setting.
	DefaultOPAWatchBundle = false

	// DefaultOPARunOnStart is the default setting for running update on start.
	DefaultOPARunOnStart = true
)

// OPACache represents caching configuration for OPA authorization decisions.
type OPACache struct {
	// Enabled enables caching of authorization decisions.
	// Improves performance for repeated authorization checks.
	// Defaults to false.
	Enabled bool `yaml:"enabled" default:"false"`

	// TTL is the time-to-live for cached decisions.
	// Defaults to 5 minutes.
	TTL time.Duration `yaml:"ttl" default:"5m"`
}

// DefaultOPACache returns an OPACache configuration with default values.
func DefaultOPACache() OPACache {
	return OPACache{
		Enabled: false,
		TTL:     DefaultOPACacheTTL,
	}
}

// Validate validates the OPACache configuration.
// It ensures that TTL is set when caching is enabled.
//
// Returns an error if any validation rules fail.
func (c *OPACache) Validate() error {
	return ValidateStruct(c,
		validation.Field(&c.TTL,
			validation.When(c.Enabled, ozzo_rules.Duration())),
	)
}

// OPA represents Open Policy Agent configuration for authorization.
// It defines settings for policy evaluation, caching, and decision logging.
//
// Example:
//
//	opa := &config.OPA{
//		BundlePath: "./policies",
//		Query:      "data.authz.allow",
//	}
type OPA struct {
	// BundlePath is the filesystem path or URL to the OPA policy bundle.
	// Can be a directory path (e.g., "./policies") or bundle URL.
	// This field is required.
	BundlePath string `yaml:"bundlePath"`

	// Query is the Rego query to evaluate for authorization decisions.
	// Defaults to "data.profiles.authz.allow".
	Query string `yaml:"query" default:"data.profiles.authz.allow"`

	// DecisionLogging enables logging of all authorization decisions.
	// Useful for auditing and debugging authorization policies.
	// Defaults to false.
	DecisionLogging bool `yaml:"decisionLogging" default:"false"`

	// Cache contains caching configuration for authorization decisions.
	// If nil, caching is disabled.
	Cache *OPACache `yaml:"cache" default:"-"`

	// WatchBundle enables automatic reloading of policies when a bundle changes.
	// Uses fsnotify to watch for filesystem changes.
	// Useful for development and dynamic policy updates.
	// Defaults to false.
	WatchBundle bool `yaml:"watchBundle" default:"false"`

	// UpdateSchedule is an optional cron schedule for periodic policy updates.
	// If set, policies are reloaded according to this schedule regardless of file changes.
	// Example: "*/5 * * * * *" for every 5 seconds, "@every 1m" for every minute.
	// Defaults to empty (disabled).
	UpdateSchedule string `yaml:"updateSchedule" default:""`

	// RunOnStart determines whether to run an update cycle when the manager starts.
	// Defaults to true.
	RunOnStart bool `yaml:"runOnStart" default:"true"`

	// FileExtensions specifies the file extensions to include when loading policies.
	// Defaults to [".rego"].
	FileExtensions []string `yaml:"fileExtensions" default:"[\".rego\"]"`

	// IncludeData enables loading .json files as OPA data.
	// When enabled, JSON files in the policy directory are loaded into OPA's data store.
	// Defaults to false.
	IncludeData bool `yaml:"includeData" default:"false"`
}

// Validate validates the OPA configuration.
// It ensures that required fields are set and validates cache settings
// when caching is enabled.
//
// Returns an error if any validation rules fail.
func (c *OPA) Validate() error {
	return ValidateStruct(c,
		validation.Field(&c.BundlePath, validation.Required),
		validation.Field(&c.Query, validation.Required),
		validation.Field(&c.Cache, validation.When(c.Cache != nil, validation.Required)),
	)
}

// DefaultOPA returns an OPA configuration with default values.
// Note: BundlePath is left as zero value since it is a required field.
func DefaultOPA() OPA {
	return OPA{
		Query:           DefaultOPAQuery,
		DecisionLogging: DefaultOPADecisionLogging,
		WatchBundle:     DefaultOPAWatchBundle,
		RunOnStart:      DefaultOPARunOnStart,
		FileExtensions:  []string{".rego"},
	}
}

// IsEnabled returns whether OPA is enabled.
// OPA is considered enabled if the configuration is not nil
// and BundlePath is not empty.
func (c *OPA) IsEnabled() bool {
	return c != nil && c.BundlePath != ""
}
