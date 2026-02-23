// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package gcp

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"regexp"
	"time"

	"github.com/googleapis/gax-go/v2"

	"github.com/altessa-s/go-atlas/core/runtime/concurrency"
	"github.com/altessa-s/go-atlas/security/secrets"
	"github.com/altessa-s/go-atlas/security/secrets/internal/base"

	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	pb "cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	corecontext "github.com/altessa-s/go-atlas/core/context"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	coreruntime "github.com/altessa-s/go-atlas/core/runtime"
	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

// Validation constants for GCP provider
const (
	maxProjectIdLength = 30 // GCP project ID max length
)

// Validation errors specific to GCP provider
var (
	ErrInvalidProjectId      = errors.New("project ID invalid: empty or exceeds length limit")
	ErrInvalidServiceAccount = errors.New("service account path invalid: cannot be empty")

	projectIdRegex      = regexp.MustCompile(`^[a-z][a-z0-9-]*[a-z0-9]$`)
	projectIdShortRegex = regexp.MustCompile(`^[a-z]$`)
	secretKeyRegex      = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_-]*$`)
	pathRx              = regexp.MustCompile(`^projects/[^/]+/secrets/(.*)$`)
	versionRx           = regexp.MustCompile(`^projects/[^/]+/secrets/(.*)/versions/([0-9]+)$`)
)

// Verify that Storage implements the secrets.Provider interface
var _ secrets.Provider[secrets.Value[any]] = (*Storage[secrets.Value[any]])(nil)

// Storage implements the secrets.Provider interface for GCP Secret Manager.
// It provides secure access to secrets stored in Google Cloud Platform's Secret Manager
// with support for concurrent operations, automatic retry logic, and flexible encoding.
//
// The Storage type is thread-safe and can be used concurrently from multiple goroutines.
// It automatically handles GCP authentication, secret name validation, and version management.
type Storage[T any] struct {
	// projectId is the GCP project ID where the secrets are stored
	projectId string

	// serviceAccountPath is the file path to the GCP service account credentials
	serviceAccountPath string

	// client is the GCP Secret Manager client for API operations
	client *secretmanager.Client

	// opts contains configuration options for the storage provider
	opts *options[T]

	// SingleflightGroup prevents thundering herd problem for concurrent List operations
	base.SingleflightGroup
}

// New creates a new GCP Secret Manager storage provider with the specified configuration.
// This function initializes the GCP Secret Manager client, configures authentication,
// and sets up regex patterns for secret name validation and parsing.
//
// The context is used for client initialization with a 10-second timeout.
// For ongoing operations, each method accepts its own context.
//
// Returns a fully configured Storage instance ready for use, or an error.
func New[T any](ctx context.Context, projectId string, serviceAccountPath string, opt ...Option[T]) (*Storage[T], error) {
	// Validate input parameters
	if err := validateProjectId(projectId); err != nil {
		return nil, err
	}

	if err := validateServiceAccountPath(serviceAccountPath); err != nil {
		return nil, err
	}

	v := &Storage[T]{
		projectId:          projectId,
		serviceAccountPath: serviceAccountPath,
		opts:               newOptions(opt...),
	}

	var err error
	if v.client, err = v.createClient(ctx); err != nil {
		return nil, err
	}

	return v, nil
}

// Name returns the provider name identifier.
// This value is used for logging, metrics, and audit trails to identify
// the storage backend being used.
//
// Returns "gcp" to indicate Google Cloud Platform Secret Manager.
func (s *Storage[T]) Name() string { return "gcp" }

// List retrieves all secrets from GCP Secret Manager that match the configured filters.
// This method performs concurrent retrieval of secret values for optimal performance,
// automatically parallelizing requests across multiple goroutines.
//
// The method applies the following filters:
//   - Secret name must match the configured prefix pattern
//   - Secret must have all required labels (if configured)
//   - Secret must be in ACTIVE state with a current version
//
// If no matching secrets are found, an empty slice is returned (not an error).
// Individual secret retrieval failures are collected and returned as a single error.
//
// Parameters:
//   - ctx: context for the operation, supports cancellation and timeouts
//
// Returns a slice of all matching secret values, or an error.
func (s *Storage[T]) List(ctx context.Context) ([]*secrets.Value[T], error) {
	ctx = corecontext.OrBackground(ctx)

	var tmp *secrets.Value[T]
	valueDecode := func(r *pb.AccessSecretVersionResponse) (*secrets.Value[T], error) {
		pathData := versionRx.FindStringSubmatch(r.Name)
		if len(pathData) != 3 { //nolint:mnd
			return tmp, fmt.Errorf("path data extraction failed: %s", r.Name)
		}

		decodedKey, err := s.opts.keyDecoder.Decode(pathData[1])
		if err != nil {
			return tmp, base.KeyDecodingError(err)
		}

		value, err := s.opts.valueDecoder.Decode(r.Payload.Data)
		if err != nil {
			return tmp, base.ValueDecodingError(err)
		}

		return &secrets.Value[T]{
			Key:          decodedKey,
			EncodedKey:   pathData[1],
			Value:        value,
			EncodedValue: r.Payload.Data,
			Version:      pathData[2],
		}, nil
	}

	list, err := s.secrets(ctx, valueDecode)
	if err != nil {
		return nil, err
	}

	return list, nil // nolint:errcheck
}

// Values returns an iterator over all secrets stored in GCP Secret Manager.
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

		// Get all secret names first
		secretNames, err := s.list(ctx)
		if err != nil {
			yield(nil, err)
			return
		}

		// Iterate through secret names and fetch values lazily
		for _, secretName := range secretNames {
			// Check context cancellation
			if ctx.Err() != nil {
				yield(nil, ctx.Err())
				return
			}

			// Fetch the latest version for this secret
			_, data, err := s.getLatestVersion(ctx, secretName)
			if err != nil {
				if errors.Is(err, secrets.ErrNotFound) {
					continue // Skip secrets without a current version
				}
				if !yield(nil, coreerrs.Wrapf(err, "latest version retrieval failed for %s", secretName)) {
					return
				}
				continue
			}

			// Decode the value
			pathData := versionRx.FindStringSubmatch(data.Name)
			if len(pathData) != 3 { //nolint:mnd
				if !yield(nil, fmt.Errorf("path data extraction failed: %s", data.Name)) {
					return
				}
				continue
			}

			decodedKey, err := s.opts.keyDecoder.Decode(pathData[1])
			if err != nil {
				if s.opts.ignoreInvalidKeys {
					continue
				}
				if !yield(nil, base.KeyDecodingError(err)) {
					return
				}
				continue
			}

			value, err := s.opts.valueDecoder.Decode(data.Payload.Data)
			if err != nil {
				if s.opts.ignoreInvalidKeys {
					continue
				}
				if !yield(nil, base.ValueDecodingError(err)) {
					return
				}
				continue
			}

			secretValue := &secrets.Value[T]{
				Key:          decodedKey,
				EncodedKey:   pathData[1],
				Value:        value,
				EncodedValue: data.Payload.Data,
				Version:      pathData[2],
			}

			if !yield(secretValue, nil) {
				return
			}
		}
	}
}

// Value retrieves a specific secret by key from GCP Secret Manager.
// The method automatically fetches the latest version of the secret and applies
// the configured key encoding before lookup.
//
// The key is encoded using the configured key decoder (typically base64) before
// being combined with the prefix to form the complete secret name in GCP.
//
// Parameters:
//   - ctx: context for the operation, supports cancellation and timeouts
//   - key: the secret key to retrieve (will be encoded before lookup)
//
// Returns the secret value with metadata, or an error.
func (s *Storage[T]) Value(ctx context.Context, key string) (*secrets.Value[T], error) {
	ctx = corecontext.OrBackground(ctx)

	if err := validateSecretKey(key); err != nil {
		return nil, coreerrs.Wrap(err, "secret key invalid")
	}

	encodedKey, err := s.opts.keyDecoder.Encode(key)
	if err != nil {
		return nil, base.KeyEncodingError(err)
	}

	// Use singleflight to prevent thundering herd for the same secret
	sfKey := s.CreateKey("gcp:secret", s.projectId, encodedKey)
	return base.DoTyped(&s.SingleflightGroup, sfKey, func() (*secrets.Value[T], error) {
		return s.doValue(ctx, key, encodedKey)
	})
}

// doValue is the actual implementation of value retrieval
// This is separated to be called through singleflight
func (s *Storage[T]) doValue(ctx context.Context, key, encodedKey string) (*secrets.Value[T], error) {
	result, err := s.client.AccessSecretVersion(ctx, &pb.AccessSecretVersionRequest{
		Name: formatSecretVersionPath(s.projectId, encodedKey),
	}, gax.WithGRPCOptions(grpc.WaitForReady(true)))

	if err != nil {
		return nil, err
	}

	pathData := versionRx.FindStringSubmatch(result.Name)
	if len(pathData) != 3 { //nolint:mnd
		return nil, fmt.Errorf("path data extraction failed: %s", result.Name)
	}

	value, err := s.opts.valueDecoder.Decode(result.Payload.Data)
	if err != nil {
		return nil, base.ValueDecodingError(err)
	}

	return &secrets.Value[T]{
		Key:          key,
		EncodedKey:   encodedKey,
		Value:        value,
		EncodedValue: result.Payload.Data,
		Version:      pathData[2],
	}, nil
}

// Delete removes a secret from GCP Secret Manager permanently.
// This operation deletes the entire secret and all of its versions, and cannot be undone.
// The secret will be immediately inaccessible and permanently removed from GCP.
//
// The method first verifies that the secret exists by attempting to retrieve its metadata.
// If the secret does not exist, secrets.ErrNotFound is returned. Only after confirmation
// does the method proceed with the permanent deletion.
//
// Parameters:
//   - ctx: context for the operation, supports cancellation and timeouts
//   - key: the key of the secret to delete (will be encoded before lookup)
//
// Returns nil if the deletion was successful, or an error.
func (s *Storage[T]) Delete(ctx context.Context, key string) error {
	ctx = corecontext.OrBackground(ctx)

	if err := validateSecretKey(key); err != nil {
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

// Save stores or updates a secret in GCP Secret Manager.
// This operation creates a new secret if it doesn't exist, or adds a new version
// to an existing secret. The value is encoded using the configured value encoder
// before being stored in GCP Secret Manager.
//
// The method performs the following steps:
//  1. Encode the key using the configured key encoder
//  2. Encode the value using JSON marshaling
//  3. Check if the secret exists, create if necessary
//  4. Add a new version with the encoded value
//
// Parameters:
//   - ctx: context for the operation, supports cancellation and timeouts
//   - key: the key of the secret to save (will be encoded before storage)
//   - value: the value to store (will be JSON-encoded)
//
// Returns nil if the save was successful, or an error.
func (s *Storage[T]) Save(ctx context.Context, key string, value T) error {
	ctx = corecontext.OrBackground(ctx)

	if err := validateSecretKey(key); err != nil {
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
	secretName := formatSecretPath(s.projectId, encodedKey)

	// Encode the value using the configured decoder
	encodedValue, err := s.opts.valueDecoder.Encode(value)
	if err != nil {
		return base.ValueEncodingError(err)
	}

	// Check if secret exists, create if it doesn't
	extSecret, err := s.client.GetSecret(ctx, &pb.GetSecretRequest{
		Name: secretName,
	}, gax.WithGRPCOptions(grpc.WaitForReady(true)))
	created := false

	if err != nil {
		st := status.Convert(err)
		if st.Code() == codes.NotFound { // Secret doesn't exist, create it
			_, err = s.client.CreateSecret(ctx, &pb.CreateSecretRequest{
				Parent:   formatProjectPath(s.projectId),
				SecretId: encodedKey,
				Secret: &pb.Secret{
					Replication: &pb.Replication{
						Replication: &pb.Replication_Automatic_{
							Automatic: &pb.Replication_Automatic{},
						},
					},
					Labels: s.opts.labels,
				},
			}, gax.WithGRPCOptions(grpc.WaitForReady(true)))

			if err != nil {
				return coreerrs.Wrap(err, "secret creation failed")
			}
			created = true
		} else {
			return coreerrs.Wrap(err, "secret existence check failed")
		}
	}

	var latestVersion *pb.SecretVersion
	if !created {
		latestVersion, _, err = s.getLatestVersion(ctx, extSecret.Name)
		if err != nil && !errors.Is(err, secrets.ErrNotFound) {
			return coreerrs.Wrap(err, "latest version retrieval failed")
		}
	}

	// Add new version
	if _, err = s.client.AddSecretVersion(ctx, &pb.AddSecretVersionRequest{
		Parent: secretName,
		Payload: &pb.SecretPayload{
			Data: encodedValue,
		},
	}, gax.WithGRPCOptions(grpc.WaitForReady(true))); err != nil {
		return coreerrs.Wrap(err, "secret version addition failed")
	}

	// If the secret was found, we can destroy the previous version.
	if latestVersion != nil {
		_, _ = s.client.DestroySecretVersion(ctx, &pb.DestroySecretVersionRequest{ //nolint:errcheck
			Name: latestVersion.Name,
			Etag: latestVersion.Etag,
		}, gax.WithGRPCOptions(grpc.WaitForReady(true)))
	}

	return nil
}

func (s *Storage[T]) delete(ctx context.Context, encodedKey string) error {
	secretName := formatSecretPath(s.projectId, encodedKey)

	// First check if the secret exists by trying to get its metadata
	_, err := s.client.GetSecret(ctx, &pb.GetSecretRequest{
		Name: secretName,
	}, gax.WithGRPCOptions(grpc.WaitForReady(true)))

	if err != nil {
		st := status.Convert(err)
		if st.Code() == codes.NotFound {
			return secrets.ErrNotFound
		}
		return coreerrs.Wrap(err, "secret existence check failed")
	}

	// Delete the secret
	err = s.client.DeleteSecret(ctx, &pb.DeleteSecretRequest{
		Name: secretName,
	}, gax.WithGRPCOptions(grpc.WaitForReady(true)))
	if err != nil {
		return coreerrs.Wrap(err, "secret deletion failed")
	}

	return nil
}

// secrets retrieves and decodes all matching secrets from GCP Secret Manager.
// This method coordinates the listing and concurrent retrieval of secret values,
// applying the provided decoder function to transform the raw GCP responses.
// Uses singleflight to prevent thundering herd when multiple goroutines call this method.
//
// Parameters:
//   - ctx: context for the operation
//   - fn: decoder function to transform GCP responses into Value objects
//
// Returns a slice of decoded secrets or an error if the operation fails.
func (s *Storage[T]) secrets(ctx context.Context, fn func(d *pb.AccessSecretVersionResponse) (*secrets.Value[T], error)) ([]*secrets.Value[T], error) {
	// Create a unique key for singleflight based on labels and project
	sfKey := s.createSingleflightKey()

	// Use singleflight to prevent multiple concurrent executions
	return base.DoTyped(&s.SingleflightGroup, sfKey, func() ([]*secrets.Value[T], error) {
		return s.doSecrets(ctx, fn)
	})
}

// doSecrets is the actual implementation of secrets retrieval
// This is separated to be called through singleflight
func (s *Storage[T]) doSecrets(ctx context.Context, fn func(d *pb.AccessSecretVersionResponse) (*secrets.Value[T], error)) ([]*secrets.Value[T], error) {
	list, err := s.list(ctx)
	if err != nil {
		return nil, err
	}

	return concurrency.ProcessCollect(ctx, list, func(ctx context.Context, name string) (*secrets.Value[T], error) {
		_, data, err := s.getLatestVersion(ctx, name)
		if err != nil {
			if errors.Is(err, secrets.ErrNotFound) {
				return nil, nil // Will be ignored if StopOnError is false
			}
			return nil, coreerrs.Wrapf(err, "latest version retrieval failed for %s", name)
		}

		value, err := fn(data)
		if err != nil {
			if s.opts.ignoreInvalidKeys {
				return nil, nil // Will be ignored if StopOnError is false
			}
			return nil, err
		}

		return value, nil
	}, concurrency.BatchConfig[string]{
		LimitFunc:   s.opts.concurrencyLimitFunc,
		StopOnError: false, // In GCP provider we always ignore nil results from not found or invalid keys
	})
}

func (s *Storage[T]) getLatestVersion(ctx context.Context, secretName string) (*pb.SecretVersion, *pb.AccessSecretVersionResponse, error) {
	latestVersion, err := s.client.GetSecretVersion(ctx,
		&pb.GetSecretVersionRequest{Name: secretName + "/versions/latest"},
		gax.WithGRPCOptions(grpc.WaitForReady(true)),
	)
	if err != nil {
		st := status.Convert(err)
		if st.Code() == codes.NotFound {
			return nil, nil, secrets.ErrNotFound
		}
		return nil, nil, coreerrs.Wrap(err, "latest version retrieval failed")
	}

	data, err := s.client.AccessSecretVersion(ctx,
		&pb.AccessSecretVersionRequest{Name: secretName + "/versions/latest"},
		gax.WithGRPCOptions(grpc.WaitForReady(true)),
	)
	if err != nil {
		return nil, nil, err
	}
	return latestVersion, data, nil
}

// list retrieves all secret names from GCP Secret Manager that match the configured filters.
// This method applies prefix matching via regex and label filtering to identify
// relevant secrets for this provider instance.
//
// Parameters:
//   - ctx: context for the operation
//
// Returns a slice of secret names (full GCP resource paths) or an error.
func (s *Storage[T]) list(ctx context.Context) ([]string, error) {
	// Get a pooled slice buffer for the result list
	listPtr := corestrings.GetStringSlice()
	defer corestrings.PutStringSlice(listPtr)
	list := *listPtr

	req := &pb.ListSecretsRequest{
		Parent: "projects/" + s.projectId,
		// Only list secrets that are not expired
		Filter: "((NOT expire_time:*) OR (expire_time:* AND expire_time>" + time.Now().UTC().Format("2006-01-02") + "))",
	}

	if len(s.opts.labels) > 0 {
		req.Filter += " AND " + buildGCPFilter(s.opts.labels)
	}

	it := s.client.ListSecrets(ctx, req, gax.WithGRPCOptions(grpc.WaitForReady(true)))
	for {
		resp, err := it.Next()
		if err != nil {
			if errors.Is(err, iterator.Done) {
				break
			}
			return nil, err
		}
		if !pathRx.MatchString(resp.Name) {
			continue
		}
		list = append(list, resp.Name)
	}

	// Create a copy of the results so the original buffer can be returned to pool
	result := make([]string, len(list))
	copy(result, list)

	return result, nil
}

// createClient initializes a new GCP Secret Manager client with the configured credentials.
// The client is configured with the service account file and telemetry disabled for security.
// A finalizer is set to ensure proper cleanup of the client connection.
//
// Returns the initialized client or an error if authentication or connection fails.
func (s *Storage[T]) createClient(ctx context.Context) (cl *secretmanager.Client, err error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second) // nolint:mnd
	defer cancel()

	cl, err = secretmanager.NewClient(ctx,
		option.WithAuthCredentialsFile(option.ServiceAccount, s.serviceAccountPath),
		option.WithTelemetryDisabled())
	if err == nil {
		coreruntime.AddCleanup(cl, func(cl *secretmanager.Client) {
			_ = cl.Close()
		}, cl)
	}
	return
}

// validateProjectId validates GCP project ID according to GCP naming rules
func validateProjectId(projectId string) error {
	if projectId == "" {
		return ErrInvalidProjectId
	}

	if len(projectId) > maxProjectIdLength {
		return ErrInvalidProjectId
	}

	// GCP project ID must contain only lowercase letters, digits, and hyphens
	// Must start with a letter, cannot end with a hyphen
	re := projectIdRegex
	if len(projectId) == 1 {
		re = projectIdShortRegex
	}

	if !re.MatchString(projectId) {
		return ErrInvalidProjectId
	}

	return nil
}

// validateServiceAccountPath validates service account file path
func validateServiceAccountPath(path string) error {
	if corestrings.IsEmptyOrWhitespace(path) {
		return ErrInvalidServiceAccount
	}
	return nil
}

// validateSecretKey validates secret key according to GCP Secret Manager rules
func validateSecretKey(key string) error {
	if !secrets.ValidateKeyLength(key) {
		return secrets.ErrInvalidKey
	}

	// Secret names must contain only alphanumeric characters, underscores, and hyphens
	// Must start with a letter or underscore
	if !secretKeyRegex.MatchString(key) {
		return secrets.ErrInvalidKey
	}

	return nil
}

// formatProjectPath formats the GCP project path for Secret Manager API requests.
func formatProjectPath(projectID string) string {
	return corestrings.Concat("projects/", projectID)
}

// formatSecretPath formats the GCP Secret Manager secret path for a given project and secret name.
func formatSecretPath(projectID, secretName string) string {
	return corestrings.Concat("projects/", projectID, "/secrets/", secretName)
}

// formatSecretVersionPath formats the GCP Secret Manager secret version path for a given project and secret name.
func formatSecretVersionPath(projectID, secretName string) string {
	return corestrings.Concat("projects/", projectID, "/secrets/", secretName, "/versions/latest")
}

// buildGCPFilter efficiently builds GCP filter strings for label filtering.
func buildGCPFilter(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}

	builder := corestrings.GetStringBuilder()
	defer corestrings.PutStringBuilder(builder)

	// Pre-calculate approximate capacity
	approxLen := 2 // for parentheses
	for k, v := range labels {
		approxLen += len("labels.") + len(k) + len("=") + len(v) + len(" AND ")
	}
	builder.Grow(approxLen)

	builder.WriteString("(")
	first := true
	for k, v := range labels {
		if !first {
			builder.WriteString(" AND ")
		}
		builder.WriteString("labels.")
		builder.WriteString(k)
		builder.WriteString("=")
		builder.WriteString(v)
		first = false
	}
	builder.WriteString(")")

	return builder.String()
}

// createSingleflightKey creates a unique key for singleflight based on project and labels
// This ensures that different configurations don't interfere with each other
func (s *Storage[T]) createSingleflightKey() string {
	if len(s.opts.labels) > 0 {
		return s.CreateKey("gcp", s.projectId, "labels", buildGCPFilter(s.opts.labels))
	}
	return s.CreateKey("gcp", s.projectId)
}

// CheckConnection verifies connectivity to GCP Secret Manager.
// This method implements the ProviderHealthChecker interface and performs
// a lightweight operation to verify that the GCP client is functioning properly.
//
// The check performs a list operation with minimal scope to verify:
//   - Service account authentication is working
//   - Network connectivity to GCP APIs is available
//   - Project permissions are correctly configured
//
// Parameters:
//   - ctx: context for the health check operation, supports cancellation and timeouts
//
// Returns nil if the connection is healthy, or an error describing the issue.
func (s *Storage[T]) CheckConnection(ctx context.Context) error {
	// Perform a lightweight list operation to verify connectivity
	req := &pb.ListSecretsRequest{
		Parent:   formatProjectPath(s.projectId),
		PageSize: 1, // Minimal page size for health check
	}

	it := s.client.ListSecrets(ctx, req)

	// Try to get the first result to verify connectivity
	_, err := it.Next()
	if err != nil {
		// If it's iterator.Done, it means the operation succeeded but no secrets exist
		if !errors.Is(err, iterator.Done) {
			return coreerrs.Wrap(err, "GCP Secret Manager connection check failed")
		}
	}

	return nil
}
