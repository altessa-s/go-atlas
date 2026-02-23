// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package vault

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"path"
	"regexp"
	"strconv"

	"github.com/altessa-s/go-atlas/core/runtime/concurrency"
	"github.com/altessa-s/go-atlas/security/secrets"
	"github.com/altessa-s/go-atlas/security/secrets/internal/base"

	coremaps "github.com/altessa-s/go-atlas/core/collections/maps"
	corecontext "github.com/altessa-s/go-atlas/core/context"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
	vaultApi "github.com/hashicorp/vault/api"
	stdSlices "slices"
)

// Validation constants for Vault provider
const (
	maxPathLength = 512 // Maximum Vault path length
)

// Validation errors specific to Vault provider
var (
	ErrNilVaultClient    = errors.New("vault client invalid: cannot be nil")
	ErrInvalidMountPath  = errors.New("mount path invalid: empty or contains invalid characters")
	ErrInvalidSecretPath = errors.New("secret path invalid: contains invalid characters")

	pathRegex = regexp.MustCompile(`^[a-zA-Z0-9_/-]+$`)
)

// Verify that Storage implements the secrets.Provider interface
var _ secrets.Provider[secrets.Value[any]] = (*Storage[secrets.Value[any]])(nil)

// Storage implements the secrets.Provider interface for HashiCorp Vault.
// It provides secure access to secrets stored in Vault's Key-Value v2 engine
// with support for concurrent operations, version management, and flexible path configuration.
//
// The Storage type is thread-safe and can be used concurrently from multiple goroutines.
// It automatically handles Vault authentication, path construction, and version tracking.
type Storage[T any] struct {
	// client is the Vault API client for all operations
	client *vaultApi.Client

	// kv is the Vault KV v2 engine client for secret operations
	kv *vaultApi.KVv2

	// opts contains configuration options for the storage provider
	opts *options[T]

	// SingleflightGroup prevents thundering herd problem for concurrent operations
	base.SingleflightGroup
}

// New creates a new Vault storage provider with the specified configuration.
// This function initializes the Vault KV v2 client and configures the storage
// with the provided options for mount path, secret path, and key prefix.
//
// The client must be properly authenticated with a valid Vault token and have
// appropriate policies configured for the target paths. Required capabilities:
//   - read (for secret retrieval and metadata access)
//   - list (for secret enumeration)
//   - delete (for secret removal operations)
//
// Returns a fully configured Storage instance ready for use.
// The instance uses the provided client's authentication and policies.
func New[T any](client *vaultApi.Client, opt ...Option[T]) (*Storage[T], error) {
	// Validate input parameters
	if err := validateVaultClient(client); err != nil {
		return nil, err
	}

	opts := newOptions(opt...)

	if err := validateVaultMountPath(opts.mountPath); err != nil {
		return nil, err
	}

	if err := validateVaultSecretPath(opts.secretPath); err != nil {
		return nil, err
	}

	v := &Storage[T]{
		client: client,
		opts:   opts,
	}

	v.kv = client.KVv2(v.opts.mountPath)

	return v, nil
}

// Name returns the provider name identifier.
// This value is used for logging, metrics, and audit trails to identify
// the storage backend being used.
//
// Returns "vault" to indicate HashiCorp Vault storage.
func (s *Storage[T]) Name() string { return "vault" }

// List retrieves all secrets from Vault that match the configured path filters.
// This method performs concurrent retrieval of secret values for optimal performance,
// automatically parallelizing requests across multiple goroutines.
//
// The method lists all keys under the configured secret path and prefix, then
// retrieves each secret's current version concurrently. Individual secret
// retrieval failures are collected and returned as a single error.
//
// If no matching secrets are found, an empty slice is returned (not an error).
// This can occur when the configured path is empty or when the Vault token
// lacks list permissions for the target path.
//
// Parameters:
//   - ctx: context for the operation, supports cancellation and timeouts
//
// Returns a slice of all matching secret values, or an error if the operation fails.
func (s *Storage[T]) List(ctx context.Context) ([]*secrets.Value[T], error) {
	ctx = corecontext.OrBackground(ctx)

	// Use singleflight to prevent thundering herd for List operations
	sfKey := s.CreateKey("vault", s.opts.mountPath, s.opts.secretPath)
	return base.DoTyped(&s.SingleflightGroup, sfKey, func() ([]*secrets.Value[T], error) {
		return s.doList(ctx)
	})
}

// doList is the actual implementation of list retrieval
// This is separated to be called through singleflight
func (s *Storage[T]) doList(ctx context.Context) ([]*secrets.Value[T], error) {
	keys, err := s.listKeys(ctx)
	if err != nil {
		return nil, err
	}

	return concurrency.ProcessCollect[string, *secrets.Value[T]](ctx, keys, s.value, concurrency.BatchConfig[string]{
		LimitFunc: s.opts.concurrencyLimitFunc,
	})
}

// Values returns an iterator over all secrets stored in Vault.
// This method provides lazy iteration for memory-efficient processing of large secret stores.
// Each secret is fetched on demand as the iterator is consumed.
//
// The iterator yields secrets one at a time without loading all secrets into memory at once.
// It respects context cancellation and stops iteration early if the context is canceled.
//
// Parameters:
//   - ctx: context for the operation, supports cancellation and timeouts
//
// Returns an iterator that yields (value, error) pairs.
func (s *Storage[T]) Values(ctx context.Context) iter.Seq2[*secrets.Value[T], error] {
	return func(yield func(*secrets.Value[T], error) bool) {
		ctx = corecontext.OrBackground(ctx)

		// Get all keys first
		keys, err := s.listKeys(ctx)
		if err != nil {
			yield(nil, err)
			return
		}

		// Iterate through keys and fetch values lazily
		for _, key := range keys {
			// Check context cancellation
			if ctx.Err() != nil {
				yield(nil, ctx.Err())
				return
			}

			// Fetch the value for this key
			value, err := s.value(ctx, key)
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}

			if !yield(value, nil) {
				return
			}
		}
	}
}

// Value retrieves a specific secret by key from Vault.
// The method automatically constructs the full Vault path using the configured
// mount path, secret path, prefix, and encoded key, then retrieves the latest version.
//
// The key is encoded using the configured key decoder before being used to
// construct the full path. The secret data is expected to be stored in a "data"
// field within the Vault secret, following KV v2 engine conventions.
//
// Parameters:
//   - ctx: context for the operation, supports cancellation and timeouts
//   - key: the secret key to retrieve (will be encoded before path construction)
//
// Returns the secret value with metadata, or an error.
func (s *Storage[T]) Value(ctx context.Context, key string) (*secrets.Value[T], error) {
	ctx = corecontext.OrBackground(ctx)

	if err := secrets.ValidateSecretKey(key); err != nil {
		return nil, coreerrs.Wrap(err, "secret key invalid")
	}

	encodedKey, err := s.opts.keyDecoder.Encode(key)
	if err != nil {
		return nil, base.KeyEncodingError(err)
	}

	// Use singleflight to prevent thundering herd for the same secret
	sfKey := s.CreateKey("vault:secret", s.opts.mountPath, s.opts.secretPath, encodedKey)
	return base.DoTyped(&s.SingleflightGroup, sfKey, func() (*secrets.Value[T], error) {
		return s.value(ctx, encodedKey)
	})
}

// value is an internal helper method that retrieves and decodes a secret value.
// This method handles the low-level Vault API interaction and value construction.
//
// Parameters:
//   - ctx: context for the operation
//   - key: the secret key (already encoded)
//
// Returns the decoded secret value or an error.
func (s *Storage[T]) value(ctx context.Context, key string) (*secrets.Value[T], error) {
	secret, err := s.kv.Get(ctx, s.path(key))
	if err != nil {
		if errors.Is(err, vaultApi.ErrSecretNotFound) {
			return nil, secrets.ErrNotFound
		}
		return nil, err
	}

	if secret == nil {
		return nil, secrets.ErrNotFound
	}

	var data string
	if val, ok := secret.Data["data"]; !ok {
		return nil, fmt.Errorf("data was not found in secret")
	} else if data, ok = val.(string); !ok || data == "" {
		return nil, fmt.Errorf("data was not found in secret")
	}

	// Decode the key to get the original key for decoding the value
	decodedKey, err := s.opts.keyDecoder.Decode(key)
	if err != nil {
		return nil, base.KeyDecodingError(err)
	}

	value, err := s.opts.valueDecoder.Decode([]byte(data))
	if err != nil {
		return nil, base.ValueDecodingError(err)
	}

	return &secrets.Value[T]{
		EncodedKey:   key,
		Key:          decodedKey,
		Value:        value,
		EncodedValue: []byte(data),
		Version:      strconv.Itoa(secret.VersionMetadata.Version),
	}, nil
}

// path constructs the full path to a secret in Vault.
// It combines the configured secret path, prefix, and the key, filtering out
// empty components to create a clean path structure.
//
// Parameters:
//   - key: the secret key to construct the path for
//
// Returns the complete path to the secret in Vault's KV v2 engine.
func (s *Storage[T]) path(key string) string {
	pathComps := []string{s.opts.secretPath, key}
	pathComps = stdSlices.DeleteFunc(pathComps, func(i string) bool {
		return i == ""
	})
	return path.Join(pathComps...)
}

// listKeys retrieves all secret keys from Vault under the configured path.
// This method queries the Vault metadata endpoint to enumerate available secrets
// and returns their keys after filtering and processing.
//
// Parameters:
//   - ctx: context for the operation
//
// Returns a slice of secret keys or an error if the operation fails.
func (s *Storage[T]) listKeys(ctx context.Context) ([]string, error) {
	secret, err := s.client.Logical().ListWithContext(ctx,
		path.Join(s.opts.mountPath, "metadata", s.opts.secretPath),
	)

	if err != nil {
		if errors.Is(err, vaultApi.ErrSecretNotFound) {
			return nil, secrets.ErrNotFound
		}
		return nil, err
	}

	if secret == nil || secret.Data == nil {
		return nil, secrets.ErrNotFound
	}

	if keys, ok := secret.Data["keys"]; !ok {
		return []string{}, nil
	} else if list, ok := keys.([]any); ok && len(list) > 0 {
		// Get a pooled slice buffer for the result
		resultPtr := corestrings.GetStringSliceWithCapacity(len(list))
		defer corestrings.PutStringSlice(resultPtr)
		result := *resultPtr

		// Process the keys using the pooled slice
		for _, i := range list {
			p := i.(string) //nolint:errcheck
			result = append(result, corestrings.TrimSuffixFast(p, "/"))
		}

		// Create a copy of the results so the original buffer can be returned to pool
		finalResult := make([]string, len(result))
		copy(finalResult, result)

		return finalResult, nil
	}

	return []string{}, nil
}

// Delete removes a secret from Vault KV v2 engine permanently.
// This operation performs a complete removal by first destroying all versions
// of the secret, then deleting the secret metadata. This is irreversible.
//
// The method uses a two-step process for complete removal:
//  1. Destroy all versions (makes data unrecoverable)
//  2. Delete metadata (removes the secret completely)
//
// After this operation, the secret path becomes available for reuse.
//
// Required Vault capabilities:
//   - read (to verify existence via metadata retrieval)
//   - delete (to destroy versions and delete metadata)
//
// Parameters:
//   - ctx: context for the operation, supports cancellation and timeouts
//   - key: the key of the secret to delete (will be encoded before path construction)
//
// Returns nil if the secret was successfully deleted, or an error.
func (s *Storage[T]) Delete(ctx context.Context, key string) error {
	ctx = corecontext.OrBackground(ctx)

	if err := secrets.ValidateSecretKey(key); err != nil {
		return coreerrs.Wrap(err, "secret key invalid")
	}

	encodedKey, err := s.opts.keyDecoder.Encode(key)
	if err != nil {
		return base.KeyEncodingError(err)
	}

	return base.WithLock(ctx, s.opts.locker, s.Name(), encodedKey, func(ctx context.Context) error {
		return s.delete(ctx, encodedKey)
	})
}

func (s *Storage[T]) delete(ctx context.Context, encodedKey string) error {
	secretPath := s.path(encodedKey)

	// First check if the secret exists by trying to get its metadata
	_, err := s.kv.GetMetadata(ctx, secretPath)
	if err != nil {
		// If the secret doesn't exist, return ErrNotFound
		if errors.Is(err, vaultApi.ErrSecretNotFound) {
			return secrets.ErrNotFound
		}
		return coreerrs.Wrap(err, "secret existence check failed")
	}

	// Destroy all versions of the secret (hard delete)
	err = s.kv.Destroy(ctx, secretPath, []int{}) // Empty slice means destroy all versions
	if err != nil {
		return coreerrs.Wrap(err, "secret destruction failed")
	}

	// Delete the secret metadata (removes the secret completely)
	err = s.kv.DeleteMetadata(ctx, secretPath)
	if err != nil {
		return coreerrs.Wrap(err, "secret metadata deletion failed")
	}

	return nil
}

// Save stores or updates a secret in Vault KV v2 engine.
// This operation creates a new secret if it doesn't exist, or adds a new version
// to an existing secret. The value is encoded using JSON marshaling before being
// stored in Vault.
//
// The method constructs the full Vault path using the configured mount path,
// secret path, prefix, and encoded key, then stores the value in the "data" field
// following KV v2 engine conventions.
//
// Required Vault capabilities:
//   - create (for new secrets)
//   - update (for updating existing secrets)
//
// Parameters:
//   - ctx: context for the operation, supports cancellation and timeouts
//   - key: the key of the secret to save (will be encoded before path construction)
//   - value: the value to store (will be JSON-encoded and stored in "data" field)
//
// Returns nil or an error if the operation fails.
func (s *Storage[T]) Save(ctx context.Context, key string, value T) error {
	ctx = corecontext.OrBackground(ctx)

	if err := secrets.ValidateSecretKey(key); err != nil {
		return coreerrs.Wrap(err, "secret key invalid")
	}

	encodedKey, err := s.opts.keyDecoder.Encode(key)
	if err != nil {
		return base.KeyEncodingError(err)
	}

	return base.WithLock(ctx, s.opts.locker, s.Name(), encodedKey, func(ctx context.Context) error {
		return s.save(ctx, encodedKey, value)
	})
}

func (s *Storage[T]) save(ctx context.Context, encodedKey string, value T) error {
	// Encode the value using the configured decoder
	encodedValue, err := s.opts.valueDecoder.Encode(value)
	if err != nil {
		return base.ValueEncodingError(err)
	}

	secretPath := s.path(encodedKey)

	currentSecretMetadata, err := s.kv.GetMetadata(ctx, secretPath)
	if err != nil {
		if !errors.Is(err, vaultApi.ErrSecretNotFound) {
			return coreerrs.Wrap(err, "secret metadata retrieval failed")
		}
	}

	// Store in Vault with "data" field
	data := map[string]any{
		"data": string(encodedValue),
	}

	_, err = s.kv.Put(ctx, secretPath, data) //nolint:errcheck
	if err != nil {
		return coreerrs.Wrap(err, "secret saving failed")
	}

	if currentSecretMetadata != nil && len(currentSecretMetadata.Versions) > 0 {
		filtered := coremaps.FilterMap(currentSecretMetadata.Versions, func(s string, metadata vaultApi.KVVersionMetadata) bool {
			return !metadata.Destroyed
		})
		versions := make([]int, 0, len(filtered))
		for _, v := range coremaps.Map(filtered, func(k string, v vaultApi.KVVersionMetadata) (string, int) {
			return k, v.Version
		}) {
			versions = append(versions, v)
		}
		stdSlices.Sort(versions)
		_ = s.kv.Destroy(ctx, secretPath, versions) //nolint:errcheck // Ensure all previous versions are destroyed
	}

	return nil
}

// validateVaultClient validates that the Vault client is not nil
func validateVaultClient(client *vaultApi.Client) error {
	if client == nil {
		return ErrNilVaultClient
	}
	return nil
}

// validateVaultMountPath validates Vault mount path
func validateVaultMountPath(mountPath string) error {
	if corestrings.IsEmptyOrWhitespace(mountPath) {
		return ErrInvalidMountPath
	}

	if len(mountPath) > maxPathLength {
		return ErrInvalidMountPath
	}

	// Mount path should be a valid path without dangerous characters
	if !pathRegex.MatchString(mountPath) {
		return ErrInvalidMountPath
	}

	return nil
}

// validateVaultSecretPath validates Vault secret path
func validateVaultSecretPath(secretPath string) error {
	if secretPath == "" {
		// Empty secret path is allowed (root path)
		return nil
	}

	if len(secretPath) > maxPathLength {
		return ErrInvalidSecretPath
	}

	// Secret path should be a valid path without dangerous characters
	if !pathRegex.MatchString(secretPath) {
		return ErrInvalidSecretPath
	}

	return nil
}

// CheckConnection verifies connectivity to HashiCorp Vault.
// This method implements the ProviderHealthChecker interface and performs
// a lightweight operation to verify that the Vault client is functioning properly.
//
// The check performs a health check operation to verify:
//   - Vault server is reachable
//   - Authentication token is valid
//   - KV engine is accessible
//   - Mount path is correctly configured
//
// Parameters:
//   - ctx: context for the health check operation, supports cancellation and timeouts
//
// Returns nil if the connection is healthy, or an error describing the issue.
func (s *Storage[T]) CheckConnection(ctx context.Context) error {
	// Check Vault server health
	health, err := s.client.Sys().HealthWithContext(ctx)
	if err != nil {
		return coreerrs.Wrap(err, "vault server health check failed")
	}

	if health.Sealed {
		return fmt.Errorf("vault server is sealed")
	}

	// Verify KV engine access by checking mount configuration
	mounts, err := s.client.Sys().ListMountsWithContext(ctx)
	if err != nil {
		return coreerrs.WrapOperation(err, "list Vault mounts")
	}

	// Check if the configured mount path exists
	mountPath := s.opts.mountPath
	if mountPath[len(mountPath)-1] != '/' {
		mountPath += "/"
	}

	if _, exists := mounts[mountPath]; !exists {
		return fmt.Errorf("KV mount path '%s' not found", s.opts.mountPath)
	}

	return nil
}
