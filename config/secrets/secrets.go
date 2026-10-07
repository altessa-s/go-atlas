// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package secretsconfig

import (
	"time"

	retryconfig "github.com/altessa-s/go-atlas/config/retry"
	validationconfig "github.com/altessa-s/go-atlas/config/validation"
	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Default values for Secrets configuration.
const (
	defaultSecretsProvider         = ProviderMemory
	defaultSecretsUpdateSchedule   = "0 */5 * * * *" // #nosec G101 -- cron schedule string (every 5 minutes), not a credential
	defaultSecretsCacheMaxSize     = 1000
	defaultSecretsRetryMaxAttempts = 3
	defaultSecretsRetryBaseDelay   = time.Second
	defaultSecretsRetryMaxDelay    = time.Minute
	defaultSecretsRetryMultiplier  = 2.0
	defaultSecretsEmptyListings    = 3
	defaultSecretsVaultMountPath   = "kv"
	defaultSecretsVaultSecretPath  = "blitz"
	defaultSecretsVaultCAS         = false
)

// Provider defines the type of secrets storage provider.
type Provider string

const (
	// ProviderMemory uses in-memory storage (for testing/development).
	ProviderMemory Provider = "memory"
	// ProviderVault uses HashiCorp Vault KV v2 engine.
	ProviderVault Provider = "vault"
	// ProviderGCP uses Google Cloud Secret Manager.
	ProviderGCP Provider = "gcp"
	// ProviderLockbox uses Yandex Cloud Lockbox.
	ProviderLockbox Provider = "lockbox"
)

// Cache configures the secrets cache behavior.
type Cache struct {
	// MaxSize is the maximum number of cached entries.
	// 0 means use default (1000).
	MaxSize int `yaml:"maxSize" default:"1000"`
	// ShardCount is the number of shards for sharded cache.
	// 0 means use standard (non-sharded) cache.
	ShardCount int `yaml:"shardCount" default:"0"`
}

// Vault contains Vault-specific secrets provider configuration.
type Vault struct {
	// MountPath is the mount path for Vault KV v2 engine.
	MountPath string `yaml:"mountPath" default:"kv"`
	// SecretPath is the base path for secrets within the KV engine.
	SecretPath string `yaml:"secretPath" default:"blitz"`
	// CAS enables Check-and-Set operations for atomic updates.
	CAS bool `yaml:"cas" default:"false"`
}

// GCP contains GCP Secret Manager-specific configuration.
type GCP struct {
	// ProjectID is the GCP project ID.
	ProjectID string `yaml:"projectID"`
	// ServiceAccountPath is the path to service account JSON file.
	ServiceAccountPath string `yaml:"serviceAccountPath"`
	// Labels filters secrets by GCP labels.
	Labels map[string]string `yaml:"labels"`
	// IgnoreInvalidKeys skips keys that fail to decode instead of returning error.
	IgnoreInvalidKeys bool `yaml:"ignoreInvalidKeys" default:"false"`
}

// Lockbox contains Yandex Cloud Lockbox-specific configuration.
type Lockbox struct {
	// FolderID is the Yandex Cloud folder ID.
	FolderID string `yaml:"folderID"`
	// KeyID is the service account key ID.
	KeyID string `yaml:"keyID"`
	// ServiceKeyID is the service account ID.
	ServiceKeyID string `yaml:"serviceKeyID"`
	// PrivateKeyPath is the path to the private key PEM file.
	PrivateKeyPath string `yaml:"privateKeyPath"`
	// Labels filters secrets by Yandex Cloud labels.
	Labels map[string]string `yaml:"labels"`
	// IgnoreInvalidKeys skips keys that fail to decode instead of returning error.
	IgnoreInvalidKeys bool `yaml:"ignoreInvalidKeys" default:"false"`
}

// Config defines the configuration for secrets management.
// Supports multiple storage providers with provider-specific settings.
//
// Example:
//
//	secrets := &secretsconfig.Config{
//		Provider: secretsconfig.ProviderVault,
//		Vault:    &secretsconfig.Vault{MountPath: "kv", SecretPath: "myapp"},
//	}
type Config struct {
	// Provider is the type of secrets storage to use.
	Provider Provider `yaml:"provider" default:"memory"`

	// UpdateSchedule is the cron schedule for cache updates.
	// Example: "0 */5 * * * *" (every 5 minutes)
	UpdateSchedule string `yaml:"updateSchedule" default:"0 */5 * * * *"`

	// RunOnStart triggers an immediate update cycle when starting.
	RunOnStart bool `yaml:"runOnStart" default:"true"`

	// EmptyListingThreshold is the number of consecutive successful empty
	// listings after which an update cycle clears the cache; fewer keep it,
	// so a provider that transiently lists nothing cannot wipe it. Zero uses
	// the manager default (3).
	EmptyListingThreshold int `yaml:"emptyListingThreshold" default:"3"`

	// AllowShallowClone accepts payloads whose copy would share mutable memory
	// with the cached value instead of rejecting them. Defaults to false.
	AllowShallowClone bool `yaml:"allowShallowClone" default:"false"`

	// Cache contains cache configuration.
	Cache Cache `yaml:"cache"`

	// Retry contains retry configuration for storage operations.
	Retry retryconfig.Config `yaml:"retry"`

	// Vault contains Vault-specific configuration.
	// Required when Provider is "vault".
	Vault *Vault `yaml:"vault"`

	// GCP contains GCP Secret Manager-specific configuration.
	// Required when Provider is "gcp".
	GCP *GCP `yaml:"gcp"`

	// Lockbox contains Yandex Cloud Lockbox-specific configuration.
	// Required when Provider is "lockbox".
	Lockbox *Lockbox `yaml:"lockbox"`
}

// Default returns a Secrets configuration with default values.
func Default() Config {
	return Config{
		Provider:              defaultSecretsProvider,
		UpdateSchedule:        defaultSecretsUpdateSchedule,
		RunOnStart:            true,
		EmptyListingThreshold: defaultSecretsEmptyListings,
		Cache: Cache{
			MaxSize: defaultSecretsCacheMaxSize,
		},
		Retry: retryconfig.Config{
			MaxAttempts: defaultSecretsRetryMaxAttempts,
			BaseDelay:   defaultSecretsRetryBaseDelay,
			MaxDelay:    defaultSecretsRetryMaxDelay,
			Multiplier:  defaultSecretsRetryMultiplier,
		},
	}
}

// DefaultVault returns a Vault configuration with default values.
func DefaultVault() Vault {
	return Vault{
		MountPath:  defaultSecretsVaultMountPath,
		SecretPath: defaultSecretsVaultSecretPath,
		CAS:        defaultSecretsVaultCAS,
	}
}

// Validate performs validation of the Secrets configuration.
// Returns an error if validation fails, nil otherwise.
func (s *Config) Validate() error {
	return validationconfig.ValidateStruct(s,
		validation.Field(&s.Provider, validation.Required, ozzo_rules.OneOf(
			ProviderMemory,
			ProviderVault,
			ProviderGCP,
			ProviderLockbox,
		)),
		validation.Field(&s.UpdateSchedule, validation.Required),
		validation.Field(&s.EmptyListingThreshold, validation.Min(0)),
		validation.Field(&s.Cache),
		validation.Field(&s.Retry),
		validation.Field(&s.Vault, validation.When(s.Provider == ProviderVault, validation.Required)),
		validation.Field(&s.GCP, validation.When(s.Provider == ProviderGCP, validation.Required)),
		validation.Field(&s.Lockbox, validation.When(s.Provider == ProviderLockbox, validation.Required)),
	)
}

// Validate performs validation of the Cache configuration.
func (c *Cache) Validate() error {
	return validationconfig.ValidateStruct(c,
		validation.Field(&c.MaxSize, validation.Min(0)),
		validation.Field(&c.ShardCount, validation.Min(0)),
	)
}

// Validate performs validation of the Vault configuration.
func (v *Vault) Validate() error {
	return validationconfig.ValidateStruct(v,
		validation.Field(&v.MountPath, validation.Required),
		validation.Field(&v.SecretPath, validation.Required),
	)
}

// Validate performs validation of the GCP configuration.
func (g *GCP) Validate() error {
	return validationconfig.ValidateStruct(g,
		validation.Field(&g.ProjectID, validation.Required),
		validation.Field(&g.ServiceAccountPath, validation.Required),
	)
}

// Validate performs validation of the Lockbox configuration.
func (l *Lockbox) Validate() error {
	return validationconfig.ValidateStruct(l,
		validation.Field(&l.FolderID, validation.Required),
		validation.Field(&l.KeyID, validation.Required),
		validation.Field(&l.ServiceKeyID, validation.Required),
		validation.Field(&l.PrivateKeyPath, validation.Required),
	)
}
