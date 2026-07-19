// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package lockbox

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"iter"
	"regexp"
	"time"

	"github.com/altessa-s/go-atlas/core/collections/slices"
	"github.com/altessa-s/go-atlas/core/runtime/concurrency"
	"github.com/altessa-s/go-atlas/security/secrets"
	"github.com/altessa-s/go-atlas/security/secrets/internal/base"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	corecontext "github.com/altessa-s/go-atlas/core/context"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	coreruntime "github.com/altessa-s/go-atlas/core/runtime"
	pb "github.com/yandex-cloud/go-genproto/yandex/cloud/lockbox/v1"
)

// Validation constants for Lockbox provider
const (
	maxFolderIdLength  = 50   // Maximum Yandex Cloud folder ID length
	maxServiceIdLength = 100  // Maximum service account ID length
	maxKeyIdLength     = 100  // Maximum key ID length
	minRSAKeyBits      = 2048 // Minimum RSA key size in bits for security
)

// Validation errors specific to Lockbox provider
var (
	ErrInvalidFolderId         = errors.New("folder ID invalid: empty or exceeds length limit")
	ErrInvalidKeyId            = errors.New("key ID invalid: empty or exceeds length limit")
	ErrInvalidServiceAccountId = errors.New("service account ID invalid: empty or exceeds length limit")
	ErrInvalidPrivateKey       = errors.New("private key invalid: empty or too short")

	folderIdRegex  = regexp.MustCompile(`^[a-zA-Z0-9-]+$`)
	keyIdRegex     = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	serviceIdRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	secretKeyRegex = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_-]*$`)
)

const entryKeyData = "Data"

// Verify that Storage implements the secrets.Provider interface
var _ secrets.Provider[secrets.Value[any]] = (*Storage[secrets.Value[any]])(nil)

// API endpoints for Yandex Cloud Lockbox service
const lockBoxAddress = "lockbox.api.cloud.yandex.net:443"         // Address for secret management operations
const payloadAddress = "payload.lockbox.api.cloud.yandex.net:443" // Address for payload retrieval operations

// decoder is a function type for decoding Lockbox payloads into secret values
type decoder[T any] func(key string, p *pb.Payload) (*secrets.Value[T], error)

// Storage implements the secrets.Provider interface for Yandex Cloud Lockbox.
// It provides secure access to secrets stored in Yandex Cloud Lockbox service
// with support for concurrent operations, IAM authentication, and advanced filtering.
//
// The Storage type is thread-safe and can be used concurrently from multiple goroutines.
// It uses a dual-client architecture with separate clients for secret management and
// payload retrieval operations, optimizing performance and security.
type Storage[T any] struct {
	// Embedding SingleflightGroup for thundering herd prevention
	base.SingleflightGroup

	// folderId is the Yandex Cloud folder ID where secrets are stored
	folderId string

	// opts contains configuration options for the storage provider
	opts *options[T]

	// ssClient is the Lockbox Secret Service client for secret management operations
	ssClient pb.SecretServiceClient

	// plClient is the Lockbox Payload Service client for retrieving secret values
	plClient pb.PayloadServiceClient

	// token manages authentication with Yandex Cloud IAM
	token *Token
}

// New creates a new Yandex Cloud Lockbox storage provider with the specified configuration.
// This function initializes the dual-client architecture (Secret Service + Payload Service),
// configures IAM authentication, and sets up regex patterns for secret name validation.
//
// The provider establishes two separate gRPC connections to Yandex Cloud:
//  1. Secret Service client for metadata operations (list, get, delete)
//  2. Payload Service client for secure value retrieval
//
// Authentication is performed using Yandex Cloud service account key authentication
// with automatic JWT token generation and refresh.
//
// The context is used for gRPC connection establishment.
// For ongoing operations, each method accepts its own context.
//
// Returns a fully configured Storage instance ready for use, or an error.
func New[T any](ctx context.Context, folderId, keyId, serviceKeyId string, privKey []byte, opt ...Option[T]) (*Storage[T], error) {
	// Validate input parameters
	if err := validateLockboxFolderId(folderId); err != nil {
		return nil, err
	}

	if err := validateLockboxKeyId(keyId); err != nil {
		return nil, err
	}

	if err := validateLockboxServiceAccountId(serviceKeyId); err != nil {
		return nil, err
	}

	if err := validateLockboxPrivateKey(privKey); err != nil {
		return nil, err
	}

	v := &Storage[T]{
		folderId: folderId,
		token:    NewLockBoxToken(keyId, serviceKeyId, privKey),
		opts:     newOptions(opt...),
	}

	scon, err := v.createClient(ctx, lockBoxAddress)
	if err != nil {
		return nil, err
	}
	v.ssClient = pb.NewSecretServiceClient(scon)

	pcon, err := v.createClient(ctx, payloadAddress)
	if err != nil {
		return nil, err
	}
	v.plClient = pb.NewPayloadServiceClient(pcon)

	return v, nil
}

// Name returns the provider name identifier.
// This value is used for logging, metrics, and audit trails to identify
// the storage backend being used.
//
// Returns "lockbox" to indicate Yandex Cloud Lockbox storage.
func (s *Storage[T]) Name() string { return "lockbox" }

// List retrieves all secrets from Yandex Cloud Lockbox that match the configured filters.
// This method performs concurrent retrieval of secret values for optimal performance,
// automatically parallelizing requests across multiple goroutines.
//
// The method applies the following filters:
//   - Secret name must match the configured prefix pattern
//   - Secret must have all required labels (if configured)
//   - Secret must be in ACTIVE state with a current version
//
// The operation uses optimized pagination and incremental filtering to handle
// large numbers of secrets efficiently without loading everything into memory.
//
// If no matching secrets are found, an empty slice is returned (not an error).
// Individual secret retrieval failures are collected and returned as a single error.
//
// Parameters:
//   - ctx: context for the operation, supports cancellation and timeouts
//
// Returns a slice of all matching secret values, or an error if the operation fails.
func (s *Storage[T]) List(ctx context.Context) ([]*secrets.Value[T], error) {
	ctx = corecontext.OrBackground(ctx)

	list, err := s.secrets(ctx, s.decodeValue)
	if err != nil {
		return nil, err
	}
	return list, nil
}

// Values returns an iterator over all secrets stored in Yandex Cloud Lockbox.
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

		// Get all secrets first
		secretList, err := s.list(ctx)
		if err != nil {
			yield(nil, err)
			return
		}

		// Iterate through secrets and fetch values lazily
		for _, secret := range secretList {
			// Check context cancellation
			if ctx.Err() != nil {
				yield(nil, ctx.Err())
				return
			}

			// Skip secrets without current version
			if secret.CurrentVersion == nil {
				continue
			}

			// Fetch the value for this secret
			value, err := s.version(ctx, secret, secret.CurrentVersion.Id, s.decodeValue)
			if err != nil {
				if s.opts.ignoreInvalidKeys {
					continue
				}
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

// Value retrieves a specific secret by key from Yandex Cloud Lockbox.
// The method automatically encodes the key and combines it with the configured
// prefix to form the complete secret name for lookup in Lockbox.
//
// The operation uses an optimized search strategy with early termination,
// minimizing API calls by searching through paginated results until the
// target secret is found.
//
// Parameters:
//   - ctx: context for the operation, supports cancellation and timeouts
//   - key: the secret key to retrieve (will be encoded before lookup)
//
// Returns the secret value with metadata, or an error.
func (s *Storage[T]) Value(ctx context.Context, key string) (*secrets.Value[T], error) {
	ctx = corecontext.OrBackground(ctx)

	if err := validateLockboxSecretKey(key); err != nil {
		return nil, coreerrs.Wrap(err, "secret key invalid")
	}

	encodedKey, err := s.opts.keyDecoder.Encode(key)
	if err != nil {
		return nil, base.KeyEncodingError(err)
	}

	// Use singleflight to prevent thundering herd for the same secret
	sfKey := s.CreateKey("lockbox", "secret", s.folderId, encodedKey)
	return base.DoTyped(&s.SingleflightGroup, sfKey, func() (*secrets.Value[T], error) {
		return s.doValue(ctx, encodedKey)
	})
}

// doValue is the actual implementation of value retrieval
// This is separated to be called through singleflight
func (s *Storage[T]) doValue(ctx context.Context, encodedKey string) (*secrets.Value[T], error) {
	secretKey := encodedKey
	secret, err := s.secretByKey(ctx, secretKey)
	if err != nil {
		if errors.Is(err, secrets.ErrNotFound) {
			return nil, secrets.ErrNotFound
		}
		return nil, err
	}

	resp, err := s.ssClient.Get(ctx, &pb.GetSecretRequest{SecretId: secret.Id}, grpc.WaitForReady(true))
	if err != nil {
		return nil, err
	}

	if resp == nil || resp.CurrentVersion == nil {
		return nil, secrets.ErrNotFound
	}

	return s.version(ctx, secret, resp.CurrentVersion.Id, s.decodeValue)
}

// secretByKey searches for a secret with the specified key name using optimized pagination.
// This method implements an efficient search strategy with early termination,
// minimizing API calls by searching through paginated results until the target secret is found.
//
// The search applies filtering incrementally to avoid loading unnecessary data:
//  1. Exact name matching before applying heavy validation
//  2. State and label validation only on matching secrets
//  3. Early termination when target secret is found
//
// Parameters:
//   - ctx: context for the operation
//   - encodedKey: the encoded secret key to search for
//
// Returns the matching secret metadata or secrets.ErrNotFound if no match is found.
func (s *Storage[T]) secretByKey(ctx context.Context, encodedKey string) (*pb.Secret, error) {
	nextPageToken := ""

	for {
		resp, err := s.ssClient.List(ctx, &pb.ListSecretsRequest{
			FolderId:  s.folderId,
			PageToken: nextPageToken,
			PageSize:  50, //nolint:mnd // Use smaller page size for faster initial response
		}, grpc.WaitForReady(true))
		if err != nil {
			return nil, err
		}

		// Search for exact match first, before applying heavy filtering
		for _, secret := range resp.Secrets {
			if secret.Name == encodedKey {
				// Apply filtering only to the found secret for validation
				if s.isValidSecret(secret) {
					return secret, nil
				}
				// If found secret doesn't pass validation, it doesn't exist for us
				return nil, secrets.ErrNotFound
			}
		}

		if resp.NextPageToken == "" {
			break
		}
		nextPageToken = resp.NextPageToken
	}
	return nil, secrets.ErrNotFound
}

// isValidSecret checks if a secret passes the configured filters.
// This method provides efficient validation for individual secrets,
// checking name patterns, state, and label requirements.
//
// Returns true if the secret meets all configured criteria, false otherwise.
func (s *Storage[T]) isValidSecret(secret *pb.Secret) bool {
	// Check status and version
	if secret.Status != pb.Secret_ACTIVE || secret.CurrentVersion == nil {
		return false
	}

	// Check labels
	for k, v := range s.opts.labels {
		if secret.Labels[k] != v {
			return false
		}
	}

	return true
}

// list retrieves all secrets from Lockbox and filters them based on configured criteria.
// This method uses optimized pagination with incremental filtering to efficiently
// handle large numbers of secrets without loading everything into memory.
//
// The filtering is applied during pagination to minimize memory usage and improve performance.
//
// Parameters:
//   - ctx: context for the operation
//
// Returns a filtered slice of secrets matching all configured criteria, or an error.
func (s *Storage[T]) list(ctx context.Context) ([]*pb.Secret, error) {
	var filteredList []*pb.Secret

	nextPageToken := ""
	for {
		resp, err := s.ssClient.List(ctx, &pb.ListSecretsRequest{
			FolderId:  s.folderId,
			PageToken: nextPageToken,
			PageSize:  100, //nolint:mnd // Use reasonable page size to balance network calls and memory
		}, grpc.WaitForReady(true))
		if err != nil {
			return nil, err
		}

		// Apply filtering incrementally to avoid storing all secrets in memory
		for _, secret := range resp.Secrets {
			filteredList = slices.AppendIf(filteredList, s.isValidSecret(secret), secret)
		}

		if resp.NextPageToken == "" {
			break
		}
		nextPageToken = resp.NextPageToken
	}

	return filteredList, nil
}

// secrets retrieves and decodes all matching secrets from Lockbox concurrently.
// This method coordinates the listing and parallel retrieval of secret values,
// using a worker pool to optimize performance while respecting API rate limits.
//
// The method uses CPU-based concurrency limits (2x CPU cores) to balance
// performance with system resource usage and API rate limits.
//
// Parameters:
//   - ctx: context for the operation
//   - fn: decoder function to transform Lockbox payloads into Value objects
//
// Returns a slice of decoded secrets or an error if the operation fails.
func (s *Storage[T]) secrets(ctx context.Context, fn decoder[T]) ([]*secrets.Value[T], error) {
	// Create a unique key for singleflight based on folder ID
	sfKey := s.CreateKey("lockbox", s.folderId)

	// Use singleflight to prevent multiple concurrent executions
	return base.DoTyped(&s.SingleflightGroup, sfKey, func() ([]*secrets.Value[T], error) {
		return s.doSecrets(ctx, fn)
	})
}

// doSecrets is the actual implementation of secrets retrieval
// This is separated to be called through singleflight
func (s *Storage[T]) doSecrets(ctx context.Context, fn decoder[T]) ([]*secrets.Value[T], error) {
	list, err := s.list(ctx)
	if err != nil {
		return nil, err
	}

	if len(list) == 0 {
		return []*secrets.Value[T]{}, nil
	}

	opts := []concurrency.Option[*pb.Secret]{
		concurrency.WithLimitFunc[*pb.Secret](s.opts.concurrencyLimitFunc),
	}
	opts = slices.AppendIf(opts, !s.opts.ignoreInvalidKeys, concurrency.WithStopOnError[*pb.Secret]())
	return concurrency.ProcessCollect(ctx, list,
		func(ctx context.Context, secret *pb.Secret) (*secrets.Value[T], error) {
			return s.version(ctx, secret, secret.CurrentVersion.Id, fn)
		},
		opts...,
	)
}

// version retrieves and decodes a specific version of a secret from Lockbox.
// This method fetches the actual secret payload using the Payload Service client
// and applies the provided decoder function to transform the data.
//
// Parameters:
//   - ctx: context for the operation
//   - secret: the secret metadata object
//   - versionId: the ID of the version to retrieve
//   - fn: decoder function to transform the payload into a Value object
//
// Returns the decoded secret value or an error if the operation fails.
func (s *Storage[T]) version(ctx context.Context, secret *pb.Secret, versionId string, fn decoder[T]) (*secrets.Value[T], error) {
	ver, err := s.plClient.Get(ctx,
		&pb.GetPayloadRequest{VersionId: versionId, SecretId: secret.Id},
		grpc.WaitForReady(true),
	)
	if err != nil {
		return nil, err
	}
	return fn(secret.Name, ver)
}

// createClient initializes a new gRPC client for communicating with Yandex Cloud Lockbox APIs.
// This method configures the connection with appropriate timeouts, keepalive settings,
// IAM authentication, and TLS security for secure communication with Yandex Cloud.
//
// A finalizer is set to ensure proper cleanup of the connection when the client is garbage collected.
//
// Parameters:
//   - ctx: context for connection establishment (reserved for future use)
//   - address: the Yandex Cloud API endpoint address to connect to
//
// Returns the initialized gRPC client connection or an error if initialization fails.
func (s *Storage[T]) createClient(_ context.Context, address string) (*grpc.ClientConn, error) {
	dialOptions := []grpc.DialOption{
		grpc.WithConnectParams(grpc.ConnectParams{MinConnectTimeout: time.Second * 5}), //nolint:mnd
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                20 * time.Second, //nolint:mnd
			Timeout:             10 * time.Second, //nolint:mnd
			PermitWithoutStream: true,             //nolint:mnd
		}),
		grpc.WithPerRPCCredentials(s.token),
		grpc.WithTransportCredentials(credentials.NewTLS(nil)),
	}

	conn, err := grpc.NewClient(address, dialOptions...)
	if err != nil {
		return nil, err
	}

	coreruntime.AddCleanup(conn, func(conn *grpc.ClientConn) {
		_ = conn.Close()
	}, conn)

	return conn, nil
}

// decodeValue is a decoder function that extracts and decodes the value from a Lockbox payload.
// This method implements the standard Lockbox payload format, looking for a "data" entry
// within the payload and applying key/value decoding as configured.
//
// The method follows Lockbox conventions where secret data is stored in an entry with
// the key "data" (case-insensitive matching).
//
// Parameters:
//   - key: the encoded key of the secret
//   - p: the Lockbox payload containing the secret data and metadata
//
// Returns the decoded secret value with complete metadata, or an error.
func (s *Storage[T]) decodeValue(key string, p *pb.Payload) (*secrets.Value[T], error) {
	var data string
	for _, entry := range p.Entries {
		if entry.Key == entryKeyData {
			data = entry.GetTextValue()
			break
		}
	}

	if data == "" {
		return nil, fmt.Errorf("failed to find data entry in payload")
	}

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
		Version:      p.VersionId,
		Value:        value,
		EncodedValue: []byte(data),
	}, nil
}

// Delete removes a secret from Yandex Cloud Lockbox permanently.
// This operation completely removes the secret and all its versions, and cannot be undone.
// The secret will be immediately inaccessible and permanently removed from Lockbox.
//
// The method first locates the secret using the same search logic as Value(),
// then performs the deletion using the Secret Service client.
//
// Parameters:
//   - ctx: context for the operation, supports cancellation and timeouts
//   - key: the key of the secret to delete (will be encoded before lookup)
//
// Returns nil if the deletion was successful, or an error.
func (s *Storage[T]) Delete(ctx context.Context, key string) error {
	ctx = corecontext.OrBackground(ctx)

	if err := validateLockboxSecretKey(key); err != nil {
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
	// Find the secret by name
	secret, err := s.secretByKey(ctx, encodedKey)
	if err != nil {
		if errors.Is(err, secrets.ErrNotFound) {
			return secrets.ErrNotFound
		}
		return err
	}

	// Delete the secret using Secret Service client
	_, err = s.ssClient.Delete(ctx, &pb.DeleteSecretRequest{
		SecretId: secret.Id,
	}, grpc.WaitForReady(true))
	if err != nil {
		return coreerrs.Wrap(err, "secret deletion failed")
	}

	return nil
}

// Save stores or updates a secret in Yandex Cloud Lockbox.
// This operation creates a new secret if it doesn't exist, or creates a new version
// of an existing secret. The value is encoded using JSON marshaling before being
// stored in Lockbox.
//
// The method performs the following steps:
//  1. Encode the key using the configured key encoder
//  2. Encode the value using JSON marshaling
//  3. Check if the secret exists, create if necessary
//  4. Create a new version with the encoded value in "data" entry
//
// Required Yandex Cloud permissions:
//   - lockbox.secrets.create (for new secrets)
//   - lockbox.versions.create (for new versions)
//   - lockbox.secrets.list (to check existence)
//
// Parameters:
//   - ctx: context for the operation, supports cancellation and timeouts
//   - key: the key of the secret to save (will be encoded before storage)
//   - value: the value to store (will be JSON-encoded)
//
// Returns nil if the save was successful, or an error.
func (s *Storage[T]) Save(ctx context.Context, key string, value T) error {
	ctx = corecontext.OrBackground(ctx)

	if err := validateLockboxSecretKey(key); err != nil {
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

	// Check if secret exists
	secret, err := s.secretByKey(ctx, encodedKey)
	if err != nil && !errors.Is(err, secrets.ErrNotFound) {
		return coreerrs.Wrap(err, "secret existence check failed")
	}

	// If secret doesn't exist, create it with initial version
	if secret == nil {
		createReq := &pb.CreateSecretRequest{
			FolderId: s.folderId,
			Name:     encodedKey,
			Labels:   s.opts.labels,
			VersionPayloadEntries: []*pb.PayloadEntryChange{
				{
					Key: entryKeyData,
					Value: &pb.PayloadEntryChange_TextValue{
						TextValue: string(encodedValue),
					},
				},
			},
			CreateVersion: wrapperspb.Bool(true),
		}

		_, err = s.ssClient.Create(ctx, createReq, grpc.WaitForReady(true))
		if err != nil {
			return coreerrs.Wrap(err, "secret creation failed")
		}
		return nil
	}

	// Create new version for existing secret
	versionReq := &pb.AddVersionRequest{
		SecretId: secret.Id,
		PayloadEntries: []*pb.PayloadEntryChange{
			{
				Key: entryKeyData,
				Value: &pb.PayloadEntryChange_TextValue{
					TextValue: string(encodedValue),
				},
			},
		},
	}

	_, err = s.ssClient.AddVersion(ctx, versionReq, grpc.WaitForReady(true))
	if err != nil {
		return coreerrs.Wrap(err, "secret version creation failed")
	}

	if secret.CurrentVersion != nil {
		_, _ = s.ssClient.ScheduleVersionDestruction(ctx, &pb.ScheduleVersionDestructionRequest{ //nolint:errcheck
			SecretId:      secret.Id,
			VersionId:     secret.CurrentVersion.Id,
			PendingPeriod: &durationpb.Duration{Seconds: int64((10 * time.Minute).Seconds())}, //nolint:mnd // Use 10 minutes pending period for version destruction
		})
	}

	return nil
}

// validateLockboxFolderId validates Yandex Cloud folder ID
func validateLockboxFolderId(folderId string) error {
	if folderId == "" {
		return ErrInvalidFolderId
	}

	if len(folderId) > maxFolderIdLength {
		return ErrInvalidFolderId
	}

	// Folder ID should contain only alphanumeric characters and hyphens
	if !folderIdRegex.MatchString(folderId) {
		return ErrInvalidFolderId
	}

	return nil
}

// validateLockboxKeyId validates Yandex Cloud key ID
func validateLockboxKeyId(keyId string) error {
	if keyId == "" {
		return ErrInvalidKeyId
	}

	if len(keyId) > maxKeyIdLength {
		return ErrInvalidKeyId
	}

	// Key ID should contain only alphanumeric characters, hyphens, and underscores
	if !keyIdRegex.MatchString(keyId) {
		return ErrInvalidKeyId
	}

	return nil
}

// validateLockboxServiceAccountId validates Yandex Cloud service account ID
func validateLockboxServiceAccountId(serviceAccountId string) error {
	if serviceAccountId == "" {
		return ErrInvalidServiceAccountId
	}

	if len(serviceAccountId) > maxServiceIdLength {
		return ErrInvalidServiceAccountId
	}

	// Service account ID should contain only alphanumeric characters, hyphens, and underscores
	if !serviceIdRegex.MatchString(serviceAccountId) {
		return ErrInvalidServiceAccountId
	}

	return nil
}

// validateLockboxPrivateKey validates private key with comprehensive checks
func validateLockboxPrivateKey(privKey []byte) error {
	if len(privKey) == 0 {
		return ErrInvalidPrivateKey
	}

	// Parse PEM block
	block, _ := pem.Decode(privKey)
	if block == nil {
		return ErrInvalidPrivateKey
	}

	// Check for supported private key types
	switch block.Type {
	case "PRIVATE KEY":
		// Parse PKCS#8 private key
		parsedKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return ErrInvalidPrivateKey
		}

		// Check if it's RSA key and validate size
		if rsaKey, ok := parsedKey.(*rsa.PrivateKey); ok {
			if rsaKey.N.BitLen() < minRSAKeyBits {
				return ErrInvalidPrivateKey
			}

			// Validate that the key is mathematically correct
			if err = rsaKey.Validate(); err != nil {
				return ErrInvalidPrivateKey
			}
		}

	default:
		// Unsupported key type
		return ErrInvalidPrivateKey
	}

	return nil
}

// validateLockboxSecretKey validates Lockbox secret key according to Yandex Cloud naming rules
func validateLockboxSecretKey(key string) error {
	if !secrets.ValidateKeyLength(key) {
		return secrets.ErrInvalidKey
	}

	// Lockbox secret names must contain only alphanumeric characters, underscores, and hyphens
	// Must start with a letter or underscore
	if !secretKeyRegex.MatchString(key) {
		return secrets.ErrInvalidKey
	}

	return nil
}

// CheckConnection verifies connectivity to Yandex Cloud Lockbox.
// This method implements the ProviderHealthChecker interface and performs
// a lightweight operation to verify that the Lockbox client is functioning properly.
//
// The check performs a list operation with minimal scope to verify:
//   - IAM authentication is working
//   - Network connectivity to Yandex Cloud APIs is available
//   - Folder permissions are correctly configured
//   - gRPC connection is functional
//
// Parameters:
//   - ctx: context for the health check operation, supports cancellation and timeouts
//
// Returns nil if the connection is healthy, or an error describing the issue.
func (s *Storage[T]) CheckConnection(ctx context.Context) error {
	// Perform a lightweight list operation to verify connectivity
	req := &pb.ListSecretsRequest{
		FolderId: s.folderId,
		PageSize: 1, // Minimal page size for health check
	}

	_, err := s.ssClient.List(ctx, req, grpc.WaitForReady(true))
	if err != nil {
		return coreerrs.Wrap(err, "yandex Cloud Lockbox connection check failed")
	}

	return nil
}

// Shutdown gracefully shuts down the Lockbox storage provider, ensuring proper cleanup
// of all background goroutines and connections. This method should be called when the
// storage provider is no longer needed to prevent goroutine leaks.
//
// The shutdown process includes:
//   - Shutting down the token manager to stop background refresh goroutines
//   - Closing gRPC connections to release network resources
//
// After calling Shutdown(), the Storage instance should not be used for further operations.
func (s *Storage[T]) Shutdown() {
	// Shutdown the token manager to stop background refresh goroutines
	if s.token != nil {
		s.token.Shutdown()
	}

	// Note: gRPC connections are typically managed by the gRPC library
	// and will be cleaned up automatically. If we need explicit connection
	// management in the future, it can be added here.
}
