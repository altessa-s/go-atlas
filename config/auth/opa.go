// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package authconfig

import (
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"strings"
	"time"

	"github.com/altessa-s/go-atlas-core/types/redacted"
	"github.com/altessa-s/go-atlas/config/internal/validators"

	httpconfig "github.com/altessa-s/go-atlas/config/http"
	proxyconfig "github.com/altessa-s/go-atlas/config/proxy"
	validationconfig "github.com/altessa-s/go-nomos/validation"
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
	// OPASourceS3 reads policies from an S3-compatible bucket.
	OPASourceS3 OPASourceProvider = "s3"
)

const (
	// DefaultOPAQuery is the default Rego query for authorization decisions.
	DefaultOPAQuery = "data.profiles.authz.allow"

	// DefaultOPACacheTTL is the default cache TTL for authorization decisions.
	DefaultOPACacheTTL = 5 * time.Minute

	// DefaultOPACacheMaxSize is the default entry cap for the decision cache.
	// It mirrors opa.DefaultDecisionCacheSize, which applies when this is left
	// at zero; the value is restated here rather than imported so that config
	// stays independent of the packages it configures.
	DefaultOPACacheMaxSize = 10000

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

	// MaxSize is the maximum number of cached decisions. The cache is an LRU,
	// so this is the memory ceiling rather than a correctness knob: a smaller
	// value evicts sooner and lowers the hit rate.
	// 0 means use the default (10000).
	MaxSize int `yaml:"maxSize" default:"10000"`
}

// DefaultOPACache returns an OPACache configuration with default values.
func DefaultOPACache() OPACache {
	return OPACache{
		Enabled: false,
		TTL:     DefaultOPACacheTTL,
		MaxSize: DefaultOPACacheMaxSize,
	}
}

// Validate validates the OPACache configuration.
// It ensures that TTL is set when caching is enabled.
//
// Returns an error if any validation rules fail.
func (c *OPACache) Validate() error {
	return validationconfig.ValidateStruct(c,
		validation.Field(&c.TTL,
			validation.When(c.Enabled, validationconfig.PositiveDuration())),
		// No Required: zero is a meaningful value here ("use the default"),
		// unlike the Min-guarded fields where it is a silent misconfiguration.
		// Min(0) still rejects a negative cap.
		validation.Field(&c.MaxSize, validation.Min(0)),
	)
}

// OPAGitLab represents GitLab-specific configuration for the OPA policy source.
type OPAGitLab struct {
	// Endpoint is the GitLab instance URL (e.g., "https://gitlab.example.com").
	// Required when source is "gitlab".
	Endpoint string `yaml:"endpoint"`

	// Token is the GitLab API access token.
	// Required when source is "gitlab".
	Token redacted.RedactedString `yaml:"token"`

	// ProjectID is the GitLab project ID.
	// Required when source is "gitlab".
	ProjectID int `yaml:"projectID"`

	// Ref is the Git ref (branch, tag, or commit SHA) to read policies from.
	// Defaults to "main".
	Ref string `yaml:"ref" default:"main"`

	// Dir is the directory path within the repository containing policy files.
	Dir string `yaml:"dir"`

	// Proxy configures the outbound HTTP proxy for GitLab API requests.
	// SSRF protection disables environment proxies. The factory rejects
	// explicit url/host modes and uses httpclient.New retry defaults.
	// Use gitlab.WithHTTPClient to supply a client with a custom retry policy.
	Proxy *proxyconfig.Config `yaml:"proxy" default:"-"`

	// SSRF configures outbound protection for GitLab API requests. The zero
	// value keeps protection enabled. AllowedCIDRs or Allow rules exempt the
	// GitLab server's network.
	SSRF *httpconfig.ClientSSRF `yaml:"ssrf" default:"-"`

	// AllowInsecureHTTP permits a plain http:// endpoint. Off by default: the
	// access token and the policies would cross the network unprotected.
	AllowInsecureHTTP bool `yaml:"allowInsecureHTTP"`

	// Checksums pins the policy and data files: file names relative to Dir
	// mapped to their lower-case SHA-256 hex digests. When set, every fetched
	// file must match its entry and every entry must be fetched. Empty
	// disables verification.
	Checksums map[string]string `yaml:"checksums"`
}

// Validate validates the OPAGitLab configuration.
func (c *OPAGitLab) Validate() error {
	return validationconfig.ValidateStruct(c,
		validation.Field(&c.Endpoint, validation.Required),
		validation.Field(&c.Token, validation.Required),
		validation.Field(&c.ProjectID, validation.Required),
		validation.Field(&c.Proxy),
		validation.Field(&c.SSRF),
		validation.Field(&c.Checksums, validation.By(validateChecksums)),
	)
}

// sha256HexPattern matches a lower-case SHA-256 hex digest, the form the
// policy sources compute and compare verbatim.
var sha256HexPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// validateChecksums rejects a checksum manifest entry with an empty file
// name or a digest that is not a lower-case SHA-256 hex string, which
// could never match.
func validateChecksums(value any) error {
	checksums, _ := value.(map[string]string) // a nil map validates
	for name, digest := range checksums {
		if name == "" {
			return errors.New("file name must not be empty")
		}
		if !sha256HexPattern.MatchString(digest) {
			return fmt.Errorf("%s: digest must be a lower-case SHA-256 hex string", name)
		}
	}
	return nil
}

// DefaultOPAGitLab returns an OPAGitLab configuration with default values.
func DefaultOPAGitLab() OPAGitLab {
	return OPAGitLab{
		Ref: "main",
	}
}

func validateReleaseManifest(value any) error {
	name, _ := value.(string)
	if name != "" && (name == "." || !fs.ValidPath(name) || strings.ContainsAny(name, "\\:\x00") || strings.TrimSpace(name) != name) {
		return errors.New("must be a relative slash-separated manifest path")
	}
	return nil
}

// OPAS3 represents S3-specific configuration for the OPA policy source.
type OPAS3 struct {
	// Bucket is the S3 bucket name containing policy files.
	// Required when source is "s3".
	Bucket string `yaml:"bucket"`

	// Prefix is the object key prefix (directory-like path) within the bucket.
	Prefix string `yaml:"prefix"`

	// ReleaseManifest is the key of the release manifest relative to Prefix.
	// Required unless Checksums supplies a static manifest.
	ReleaseManifest string `yaml:"releaseManifest"`

	// Region is the AWS region for the S3 bucket.
	Region string `yaml:"region"`

	// Endpoint is the S3-compatible endpoint URL (e.g., for MinIO).
	// Optional; defaults to the standard AWS S3 endpoint. Must be https://
	// unless AllowInsecureHTTP is set.
	Endpoint string `yaml:"endpoint"`

	// AllowInsecureHTTP permits a plain http:// endpoint. Off by default:
	// SigV4 protects the credentials but not the policy content, which a
	// MITM could swap.
	AllowInsecureHTTP bool `yaml:"allowInsecureHTTP"`

	// AccessKey is the AWS access key ID.
	// Optional; defaults to environment/instance credentials.
	AccessKey redacted.RedactedString `yaml:"accessKey"`

	// SecretKey is the AWS secret access key.
	// Optional; defaults to environment/instance credentials.
	SecretKey redacted.RedactedString `yaml:"secretKey"`

	// PathStyle enables path-style addressing (required for MinIO and some S3-compatible stores).
	PathStyle bool `yaml:"pathStyle"`

	// Proxy configures the outbound HTTP proxy for S3 API requests.
	// An empty mode with no SSRF block uses the AWS SDK's default transport,
	// which honors HTTP_PROXY/HTTPS_PROXY/NO_PROXY. Mode none uses the Atlas
	// client without a proxy. Explicit url/host modes return ErrSSRFUnsafeTransport.
	Proxy *proxyconfig.Config `yaml:"proxy" default:"-"`

	// SSRF configures outbound protection for S3 API requests. A provided
	// block selects the Atlas client, and its zero value keeps protection
	// enabled. AllowedCIDRs or Allow rules exempt the S3 endpoint's network.
	SSRF *httpconfig.ClientSSRF `yaml:"ssrf" default:"-"`

	// Checksums pins the policy and data objects: object keys relative to
	// Prefix mapped to their lower-case SHA-256 hex digests. When set, every
	// fetched object must match its entry and every entry must be fetched.
	// Use either Checksums or ReleaseManifest.
	Checksums map[string]string `yaml:"checksums"`
}

// Validate validates the OPAS3 configuration.
func (c *OPAS3) Validate() error {
	return validationconfig.ValidateStruct(c,
		validation.Field(&c.Bucket, validation.Required),
		validation.Field(&c.ReleaseManifest,
			validation.When(c.Checksums == nil, validation.Required),
			validation.When(c.Checksums != nil, validation.Empty),
			validation.By(validateReleaseManifest)),
		validation.Field(&c.Endpoint, validators.HTTPSURL(c.AllowInsecureHTTP, false)),
		validation.Field(&c.Proxy),
		validation.Field(&c.SSRF),
		validation.Field(&c.Checksums, validation.By(validateChecksums)),
	)
}

// OPA represents Open Policy Agent configuration for authorization.
// It defines settings for policy evaluation, caching, and decision logging.
//
// Example (filesystem source):
//
//	opa := &authconfig.OPA{
//		BundlePath:      "./policies",
//		ReleaseManifest: "release.json",
//		Query:           "data.authz.allow",
//	}
//
// Example (gitlab source):
//
//	opa := &authconfig.OPA{
//		Source: authconfig.OPASourceGitLab,
//		Query:  "data.authz.allow",
//		GitLab: &authconfig.OPAGitLab{
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

	// ReleaseManifest is the filesystem manifest path relative to BundlePath
	// (or the parent directory when BundlePath names a file).
	ReleaseManifest string `yaml:"releaseManifest"`

	// GitLab contains GitLab-specific configuration.
	// Required when source is "gitlab".
	GitLab *OPAGitLab `yaml:"gitlab" default:"-"`

	// S3 contains S3-specific configuration.
	// Required when source is "s3".
	S3 *OPAS3 `yaml:"s3" default:"-"`

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
	// A scheduler registrar is required when WatchBundle is false.
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
	return validationconfig.ValidateStruct(c,
		validation.Field(&c.Source, validation.Required, ozzo_rules.OneOf(
			OPASourceFilesystem,
			OPASourceGitLab,
			OPASourceEmbed,
			OPASourceS3,
		)),
		validation.Field(&c.BundlePath,
			validation.When(c.Source == "" || c.Source == OPASourceFilesystem, validation.Required)),
		validation.Field(&c.ReleaseManifest,
			validation.When(c.BundlePath != "" && (c.Source == "" || c.Source == OPASourceFilesystem),
				validation.Required, validation.By(validateReleaseManifest))),
		validation.Field(&c.GitLab,
			validation.When(c.Source == OPASourceGitLab, validation.Required)),
		validation.Field(&c.S3,
			validation.When(c.Source == OPASourceS3, validation.Required)),
		validation.Field(&c.Query, validation.Required),
		validation.Field(&c.Cache, validation.When(c.Cache != nil, validation.Required)),
		validation.Field(&c.PollInterval,
			validation.When(c.PollInterval > 0, validationconfig.PositiveDuration())),
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
	case OPASourceS3:
		return c.S3 != nil
	case OPASourceEmbed:
		return true
	default:
		return c.BundlePath != ""
	}
}
