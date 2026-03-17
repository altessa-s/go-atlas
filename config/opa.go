// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"time"

	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// OPASourceProvider defines the type of OPA policy source.
type OPASourceProvider string

const (
	// OPASourceFilesystem reads policies from the local filesystem.
	OPASourceFilesystem OPASourceProvider = "filesystem"
	// OPASourceGitLab reads policies from a GitLab repository.
	OPASourceGitLab OPASourceProvider = "gitlab"
	// OPASourceEmbed reads policies from an embedded fs.FS.
	OPASourceEmbed OPASourceProvider = "embed"
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

	// DefaultOPAPollInterval is the default polling interval for policy change detection.
	DefaultOPAPollInterval = 30 * time.Second
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

// OPAGitLab represents GitLab-specific configuration for the OPA policy source.
type OPAGitLab struct {
	// Endpoint is the GitLab instance URL (e.g., "https://gitlab.example.com").
	// Required when source is "gitlab".
	Endpoint string `yaml:"endpoint"`

	// Token is the GitLab API access token.
	// Required when source is "gitlab".
	Token Secret `yaml:"token"`

	// ProjectID is the GitLab project ID.
	// Required when source is "gitlab".
	ProjectID int `yaml:"projectID"`

	// Ref is the Git ref (branch, tag, or commit SHA) to read policies from.
	// Defaults to "main".
	Ref string `yaml:"ref" default:"main"`

	// Dir is the directory path within the repository containing policy files.
	Dir string `yaml:"dir"`

	// RetryMax is the maximum number of retry attempts for HTTP requests.
	RetryMax int `yaml:"retryMax"`

	// RetryWaitMin is the minimum wait time between retries.
	RetryWaitMin time.Duration `yaml:"retryWaitMin"`

	// RetryWaitMax is the maximum wait time between retries.
	RetryWaitMax time.Duration `yaml:"retryWaitMax"`
}

// Validate validates the OPAGitLab configuration.
func (c *OPAGitLab) Validate() error {
	return ValidateStruct(c,
		validation.Field(&c.Endpoint, validation.Required),
		validation.Field(&c.Token, validation.Required),
		validation.Field(&c.ProjectID, validation.Required),
	)
}

// DefaultOPAGitLab returns an OPAGitLab configuration with default values.
func DefaultOPAGitLab() OPAGitLab {
	return OPAGitLab{
		Ref: "main",
	}
}

// OPA represents Open Policy Agent configuration for authorization.
// It defines settings for policy evaluation, caching, and decision logging.
//
// Example (filesystem source):
//
//	opa := &config.OPA{
//		BundlePath: "./policies",
//		Query:      "data.authz.allow",
//	}
//
// Example (gitlab source):
//
//	opa := &config.OPA{
//		Source: config.OPASourceGitLab,
//		Query:  "data.authz.allow",
//		GitLab: &config.OPAGitLab{
//			Endpoint:  "https://gitlab.example.com",
//			Token:     "glpat-...",
//			ProjectID: 42,
//			Dir:       "policies/opa",
//		},
//	}
type OPA struct {
	// Source selects the policy source provider.
	// Defaults to "filesystem".
	Source OPASourceProvider `yaml:"source" default:"filesystem"`

	// BundlePath is the filesystem path to the OPA policy bundle.
	// Required when source is "filesystem".
	BundlePath string `yaml:"bundlePath"`

	// GitLab contains GitLab-specific configuration.
	// Required when source is "gitlab".
	GitLab *OPAGitLab `yaml:"gitlab" default:"-"`

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
	// The Manager polls the source at PollInterval for changes.
	// Defaults to false.
	WatchBundle bool `yaml:"watchBundle" default:"false"`

	// PollInterval is the interval between polling the source for policy changes.
	// Applies when WatchBundle is true.
	// Defaults to 30s.
	PollInterval time.Duration `yaml:"pollInterval" default:"30s"`

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
// It ensures that required fields are set and validates cache and source-specific settings.
//
// Returns an error if any validation rules fail.
func (c *OPA) Validate() error {
	return ValidateStruct(c,
		validation.Field(&c.Source, validation.Required, ozzo_rules.OneOf(
			OPASourceFilesystem,
			OPASourceGitLab,
			OPASourceEmbed,
		)),
		validation.Field(&c.BundlePath,
			validation.When(c.Source == "" || c.Source == OPASourceFilesystem, validation.Required)),
		validation.Field(&c.GitLab,
			validation.When(c.Source == OPASourceGitLab, validation.Required)),
		validation.Field(&c.Query, validation.Required),
		validation.Field(&c.Cache, validation.When(c.Cache != nil, validation.Required)),
		validation.Field(&c.PollInterval,
			validation.When(c.PollInterval > 0, ozzo_rules.Duration())),
	)
}

// DefaultOPA returns an OPA configuration with default values.
// Note: BundlePath is left as zero value since it is a required field.
func DefaultOPA() OPA {
	return OPA{
		Source:          OPASourceFilesystem,
		Query:           DefaultOPAQuery,
		DecisionLogging: DefaultOPADecisionLogging,
		WatchBundle:     DefaultOPAWatchBundle,
		PollInterval:    DefaultOPAPollInterval,
		RunOnStart:      DefaultOPARunOnStart,
		FileExtensions:  []string{".rego"},
	}
}

// IsEnabled returns whether OPA is enabled.
// OPA is considered enabled if the configuration is not nil and a source is configured.
func (c *OPA) IsEnabled() bool {
	if c == nil {
		return false
	}

	switch c.Source {
	case OPASourceGitLab:
		return c.GitLab != nil
	case OPASourceEmbed:
		return true
	default:
		return c.BundlePath != ""
	}
}
