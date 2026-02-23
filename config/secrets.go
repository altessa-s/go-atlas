// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"time"

	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Default values for Secrets configuration.
const (
	defaultSecretsProvider         = SecretsProviderMemory
	defaultSecretsUpdateSchedule   = "0 */5 * * * *" // every 5 minutes
	defaultSecretsCacheMaxSize     = 1000
	defaultSecretsRetryMaxAttempts = 3
	defaultSecretsRetryBaseDelay   = time.Second
	defaultSecretsRetryMaxDelay    = time.Minute
	defaultSecretsRetryMultiplier  = 2.0
	defaultSecretsVaultMountPath   = "kv"
	defaultSecretsVaultSecretPath  = "blitz"
	defaultSecretsVaultCAS         = false
)

// SecretsProvider defines the type of secrets storage provider.
type SecretsProvider string

const (
	// SecretsProviderMemory uses in-memory storage (for testing/development).
	SecretsProviderMemory SecretsProvider = "memory"
	// SecretsProviderVault uses HashiCorp Vault KV v2 engine.
	SecretsProviderVault SecretsProvider = "vault"
	// SecretsProviderGCP uses Google Cloud Secret Manager.
	SecretsProviderGCP SecretsProvider = "gcp"
	// SecretsProviderLockbox uses Yandex Cloud Lockbox.
	SecretsProviderLockbox SecretsProvider = "lockbox"
)

// SecretsRetry configures retry behavior for failed storage operations.
type SecretsRetry struct {
	// MaxAttempts is the maximum number of retry attempts.
	MaxAttempts int `yaml:"maxAttempts" default:"3"`
	// BaseDelay is the initial delay between retries.
	BaseDelay time.Duration `yaml:"baseDelay" default:"1s"`
	// MaxDelay is the maximum delay between retries.
	MaxDelay time.Duration `yaml:"maxDelay" default:"1m"`
	// Multiplier is the exponential backoff factor.
	Multiplier float64 `yaml:"multiplier" default:"2.0"`
}

// SecretsCache configures the secrets cache behavior.
type SecretsCache struct {
	// MaxSize is the maximum number of cached entries.
	// 0 means use default (1000).
	MaxSize int `yaml:"maxSize" default:"1000"`
	// ShardCount is the number of shards for sharded cache.
	// 0 means use standard (non-sharded) cache.
	ShardCount int `yaml:"shardCount" default:"0"`
}

// SecretsVault contains Vault-specific secrets provider configuration.
type SecretsVault struct {
	// MountPath is the mount path for Vault KV v2 engine.
	MountPath string `yaml:"mountPath" default:"kv"`
	// SecretPath is the base path for secrets within the KV engine.
	SecretPath string `yaml:"secretPath" default:"blitz"`
	// CAS enables Check-and-Set operations for atomic updates.
	CAS bool `yaml:"cas" default:"false"`
}

// SecretsGCP contains GCP Secret Manager-specific configuration.
type SecretsGCP struct {
	// ProjectID is the GCP project ID.
	ProjectID string `yaml:"projectId"`
	// ServiceAccountPath is the path to service account JSON file.
	ServiceAccountPath string `yaml:"serviceAccountPath"`
	// Labels filters secrets by GCP labels.
	Labels map[string]string `yaml:"labels"`
	// IgnoreInvalidKeys skips keys that fail to decode instead of returning error.
	IgnoreInvalidKeys bool `yaml:"ignoreInvalidKeys" default:"false"`
}

// SecretsLockbox contains Yandex Cloud Lockbox-specific configuration.
type SecretsLockbox struct {
	// FolderID is the Yandex Cloud folder ID.
	FolderID string `yaml:"folderId"`
	// KeyID is the service account key ID.
	KeyID string `yaml:"keyId"`
	// ServiceKeyID is the service account ID.
	ServiceKeyID string `yaml:"serviceKeyId"`
	// PrivateKeyPath is the path to the private key PEM file.
	PrivateKeyPath string `yaml:"privateKeyPath"`
	// Labels filters secrets by Yandex Cloud labels.
	Labels map[string]string `yaml:"labels"`
	// IgnoreInvalidKeys skips keys that fail to decode instead of returning error.
	IgnoreInvalidKeys bool `yaml:"ignoreInvalidKeys" default:"false"`
}

// Secrets defines the configuration for secrets management.
// Supports multiple storage providers with provider-specific settings.
//
// Example:
//
//	secrets := &config.Secrets{
//		Provider: config.SecretsProviderVault,
//		Vault:    &config.SecretsVault{MountPath: "kv", SecretPath: "myapp"},
//	}
type Secrets struct {
	// Provider is the type of secrets storage to use.
	Provider SecretsProvider `yaml:"provider" default:"memory"`

	// UpdateSchedule is the cron schedule for cache updates.
	// Example: "0 */5 * * * *" (every 5 minutes)
	UpdateSchedule string `yaml:"updateSchedule" default:"0 */5 * * * *"`

	// RunOnStart triggers an immediate update cycle when starting.
	RunOnStart bool `yaml:"runOnStart" default:"true"`

	// Cache contains cache configuration.
	Cache SecretsCache `yaml:"cache"`

	// Retry contains retry configuration for storage operations.
	Retry SecretsRetry `yaml:"retry"`

	// Vault contains Vault-specific configuration.
	// Required when Provider is "vault".
	Vault *SecretsVault `yaml:"vault"`

	// GCP contains GCP Secret Manager-specific configuration.
	// Required when Provider is "gcp".
	GCP *SecretsGCP `yaml:"gcp"`

	// Lockbox contains Yandex Cloud Lockbox-specific configuration.
	// Required when Provider is "lockbox".
	Lockbox *SecretsLockbox `yaml:"lockbox"`
}

// DefaultSecrets returns a Secrets configuration with default values.
func DefaultSecrets() Secrets {
	return Secrets{
		Provider:       defaultSecretsProvider,
		UpdateSchedule: defaultSecretsUpdateSchedule,
		RunOnStart:     true,
		Cache: SecretsCache{
			MaxSize: defaultSecretsCacheMaxSize,
		},
		Retry: SecretsRetry{
			MaxAttempts: defaultSecretsRetryMaxAttempts,
			BaseDelay:   defaultSecretsRetryBaseDelay,
			MaxDelay:    defaultSecretsRetryMaxDelay,
			Multiplier:  defaultSecretsRetryMultiplier,
		},
	}
}

// DefaultSecretsVault returns a SecretsVault configuration with default values.
func DefaultSecretsVault() SecretsVault {
	return SecretsVault{
		MountPath:  defaultSecretsVaultMountPath,
		SecretPath: defaultSecretsVaultSecretPath,
		CAS:        defaultSecretsVaultCAS,
	}
}

// Validate performs validation of the Secrets configuration.
// Returns an error if validation fails, nil otherwise.
func (s *Secrets) Validate() error {
	return ValidateStruct(s,
		validation.Field(&s.Provider, validation.Required, ozzo_rules.OneOf(
			SecretsProviderMemory,
			SecretsProviderVault,
			SecretsProviderGCP,
			SecretsProviderLockbox,
		)),
		validation.Field(&s.UpdateSchedule, validation.Required),
		validation.Field(&s.Cache),
		validation.Field(&s.Retry),
		validation.Field(&s.Vault, validation.When(s.Provider == SecretsProviderVault, validation.Required)),
		validation.Field(&s.GCP, validation.When(s.Provider == SecretsProviderGCP, validation.Required)),
		validation.Field(&s.Lockbox, validation.When(s.Provider == SecretsProviderLockbox, validation.Required)),
	)
}

// Validate performs validation of the SecretsRetry configuration.
func (r *SecretsRetry) Validate() error {
	return ValidateStruct(r,
		validation.Field(&r.MaxAttempts, validation.Min(0)),
		validation.Field(&r.BaseDelay, validation.Min(time.Millisecond)),
		validation.Field(&r.MaxDelay, validation.Min(time.Millisecond)),
		validation.Field(&r.Multiplier, validation.Min(1.0)),
	)
}

// Validate performs validation of the SecretsCache configuration.
func (c *SecretsCache) Validate() error {
	return ValidateStruct(c,
		validation.Field(&c.MaxSize, validation.Min(0)),
		validation.Field(&c.ShardCount, validation.Min(0)),
	)
}

// Validate performs validation of the SecretsGCP configuration.
func (g *SecretsGCP) Validate() error {
	return ValidateStruct(g,
		validation.Field(&g.ProjectID, validation.Required),
		validation.Field(&g.ServiceAccountPath, validation.Required),
	)
}

// Validate performs validation of the SecretsLockbox configuration.
func (l *SecretsLockbox) Validate() error {
	return ValidateStruct(l,
		validation.Field(&l.FolderID, validation.Required),
		validation.Field(&l.KeyID, validation.Required),
		validation.Field(&l.ServiceKeyID, validation.Required),
		validation.Field(&l.PrivateKeyPath, validation.Required),
	)
}
