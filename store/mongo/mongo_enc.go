// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// This file contains all encryption-related functionality for MongoDB
// Client-Side Field Level Encryption (CSFLE) support.

package mongo

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"reflect"
	"sync"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/store/mongo/internal/document"
	"github.com/altessa-s/go-atlas/store/mongo/kms"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	"github.com/altessa-s/go-atlas-core/runtime/panics"
	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

type fieldEncryption struct {
	provider        kms.MasterKeyProvider
	vaultDatabase   string
	vaultCollection string
	registry        *bson.Registry

	mu      sync.Mutex
	current *encryptionClientState
}

// Each connection owns its client and cache until its admitted operations finish.
type encryptionClientState struct {
	provider kms.MasterKeyProvider
	registry *bson.Registry
	client   clientEncryption
	dkids    sync.Map
	active   sync.WaitGroup
	lifetime context.Context
	cancel   context.CancelFunc
}

// clientEncryption is the subset of [mongo.ClientEncryption] the package
// uses. It is an interface so the key-vault and encryption paths can be
// exercised without libmongocrypt.
type clientEncryption interface {
	GetKeyByAltName(ctx context.Context, keyAltName string) *mongo.SingleResult
	CreateDataKey(ctx context.Context, kmsProvider string, opts ...mongoOptions.Lister[mongoOptions.DataKeyOptions]) (bson.Binary, error)
	Encrypt(ctx context.Context, val bson.RawValue, opts ...mongoOptions.Lister[mongoOptions.EncryptOptions]) (bson.Binary, error)
	Close(ctx context.Context) error
}

var _ clientEncryption = (*mongo.ClientEncryption)(nil)

func (m *fieldEncryption) open(ctx context.Context, client *mongo.Client) error {
	if m.provider == nil {
		return nil
	}
	// Create collection for key vault. This is a one-time operation.
	// The key vault collection should be created before using encryption.
	if err := createKeyVaultCollection(ctx, client.Database(m.vaultDatabase), m.vaultCollection); err != nil {
		return err
	}

	clientEncryptionOpts := mongoOptions.ClientEncryption().
		SetKeyVaultNamespace(m.vaultDatabase + "." + m.vaultCollection).
		SetKmsProviders(m.provider.Credentials())

	if tlsProvider, ok := m.provider.(kms.TLSConfigurer); ok {
		if tlsConfig := tlsProvider.TLSConfig(); tlsConfig != nil {
			clientEncryptionOpts.SetTLSConfig(map[string]*tls.Config{
				m.provider.Name(): tlsConfig,
			})
		}
	}

	// Use the main client for encryption operations (MongoDB Go driver explicitly supports this)
	ce, err := mongo.NewClientEncryption(client, clientEncryptionOpts)
	if err != nil {
		return err
	}
	m.installClient(ce)
	return nil
}

func (m *fieldEncryption) installClient(client clientEncryption) {
	lifetime, cancel := context.WithCancel(context.Background())
	m.mu.Lock()
	defer m.mu.Unlock()
	m.current = &encryptionClientState{
		provider: m.provider, registry: m.registry, client: client,
		lifetime: lifetime, cancel: cancel,
	}
}

func (m *fieldEncryption) close(ctx context.Context) error {
	m.mu.Lock()
	state := m.current
	m.current = nil
	m.mu.Unlock()
	if state == nil {
		return nil
	}
	defer state.cancel()

	// The driver may stay in a KMS call after cancellation. Finish closing
	// its client when the last operation exits, without racing the driver.
	closed := make(chan error, 1)
	go func() {
		defer panics.Handle(ctx)
		state.active.Wait()
		closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), DefaultCloseTimeout)
		defer cancel()
		closed <- state.client.Close(closeCtx)
	}()

	select {
	case err := <-closed:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func withEncryption[T any](ctx context.Context, m *fieldEncryption, op func(context.Context, *encryptionClientState) (T, error)) (T, error) {
	var zero T
	m.mu.Lock()
	state := m.current
	if state != nil {
		state.active.Add(1)
	}
	m.mu.Unlock()
	if state == nil {
		return zero, ErrEncryptionNotEnabled
	}
	defer state.active.Done()

	opCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(state.lifetime, cancel)
	defer stop()
	if state.lifetime.Err() != nil {
		cancel()
	}
	if err := opCtx.Err(); err != nil {
		return zero, err
	}
	return op(opCtx, state)
}

// EncryptionAlg represents the encryption algorithm used for field-level encryption.
type EncryptionAlg string

// String returns the string representation of the EncryptionAlg.
func (e EncryptionAlg) String() string {
	return string(e)
}

// IsValid reports whether e is a recognized encryption algorithm.
func (e EncryptionAlg) IsValid() bool {
	return e == EncryptionAlgDeterministic || e == EncryptionAlgRandom
}

// EncryptionAlgFromAlias converts a user-friendly alias to the full EncryptionAlg.
// Supported aliases are "deterministic" and "random" (case-insensitive).
// Returns an error if the alias is not recognized.
func EncryptionAlgFromAlias(alias string) (EncryptionAlg, error) {
	alg, err := document.Algorithm(alias)
	return EncryptionAlg(alg), err
}

// Encryption algorithms supported by MongoDB Client-Side Field Level Encryption (CSFLE)
const (
	// EncryptionAlgDeterministic provides deterministic encryption (same input = same output)
	// Use for fields that need to be queried (indexed, searched, sorted)
	EncryptionAlgDeterministic EncryptionAlg = document.Deterministic
	// EncryptionAlgRandom provides randomized encryption (same input = different output)
	// Use for fields that don't need to be queried but need maximum security
	EncryptionAlgRandom EncryptionAlg = document.Random
)

// DataKeyId represents a MongoDB data encryption key identifier.
type DataKeyId bson.Binary

// String returns the string representation of the DataKeyId.
func (d DataKeyId) String() string {
	return fmt.Sprintf("DataKeyId(%x)", bson.Binary(d).Data)
}

// IsEncryptionConfigured returns true if encryption is configured and available.
// This helper function checks if encryption is enabled and a KMS provider is configured.
func (m *Mongo) IsEncryptionConfigured() bool {
	return m.encryption.provider != nil
}

// DataKey retrieves or creates a data encryption key by its alternative name.
// Data keys are used for Client-Side Field Level Encryption (CSFLE) and are
// stored in the configured key vault collection.
//
// The method first checks the local cache, then queries the key vault collection.
// If the key doesn't exist and create is true, it will create a new data key.
//
// Parameters:
//   - ctx: Context for controlling operation timeout and cancellation
//   - altName: Alternative name (alias) for the data key
//   - create: Whether to create the key if it doesn't exist
//
// Returns:
//   - *DataKeyId: The data key identifier, or nil if not found and create is false
//   - error: Error if encryption is not enabled, key not found, or database error occurs
//
// Example:
//
//	// Get or create a data key for user email encryption
//	dataKey, err := mongo.DataKey(ctx, "user_email_key", true)
//	if err != nil {
//		return err
//	}
func (m *Mongo) DataKey(ctx context.Context, altName string, create bool) (*DataKeyId, error) {
	return withEncryption(ctx, &m.encryption, func(ctx context.Context, state *encryptionClientState) (*DataKeyId, error) {
		return state.dataKey(ctx, altName, create)
	})
}

func (m *encryptionClientState) dataKey(ctx context.Context, altName string, create bool) (*DataKeyId, error) {
	dkId, ok := m.dkids.Load(altName)
	if ok {
		if ret, ok := dkId.(DataKeyId); ok {
			return &ret, nil
		}
		// If type assertion failed, continue to fetch from database
	}

	ret, err := m.lookupDataKey(ctx, altName)
	if err == nil {
		m.dkids.Store(altName, *ret)
		return ret, nil
	}
	if !errors.Is(err, ErrDataKeyNotFound) || !create {
		return nil, err
	}

	return m.createDataKey(ctx, altName)
}

// CreateDataKey creates a new data encryption key with the specified alternative name.
// If a key with the same alternative name already exists, it returns the existing key;
// when a concurrent creator wins the race for altName, the winner's key is returned.
//
// The key is created using the configured KMS provider and stored in the key vault
// collection. The key ID is cached locally for future operations.
//
// Parameters:
//   - ctx: Context for controlling operation timeout and cancellation
//   - altName: Alternative name (alias) for the new data key
//
// Returns:
//   - *DataKeyId: The created or existing data key identifier
//   - error: Error if encryption is not enabled or key creation fails
func (m *Mongo) CreateDataKey(ctx context.Context, altName string) (*DataKeyId, error) {
	return withEncryption(ctx, &m.encryption, func(ctx context.Context, state *encryptionClientState) (*DataKeyId, error) {
		return state.createDataKey(ctx, altName)
	})
}

func (m *encryptionClientState) createDataKey(ctx context.Context, altName string) (*DataKeyId, error) {
	ret, err := m.lookupDataKey(ctx, altName)
	if err == nil {
		return ret, nil
	}
	if !errors.Is(err, ErrDataKeyNotFound) {
		return nil, err
	}

	// Create a new data key for the encrypted field.
	dataKeyOpts := mongoOptions.DataKey().
		SetKeyAltNames([]string{altName})

	if masterKey := m.provider.MasterKey(); masterKey != nil {
		dataKeyOpts.SetMasterKey(masterKey)
	}

	keyId, err := m.client.CreateDataKey(ctx, m.provider.Name(), dataKeyOpts)
	if err != nil {
		serr, ok := errors.AsType[mongo.ServerError](err)
		if !ok || !serr.HasErrorCode(MongoErrorCodeDuplicateKey) {
			return nil, err
		}
		// A concurrent creator stored altName first. The driver returns a
		// zero id for the losing insert, so load the winner's key instead.
		if ret, err = m.lookupDataKey(ctx, altName); err != nil {
			return nil, coreerrs.Wrapf(err, "load data key %q created concurrently", altName)
		}
	} else {
		if len(keyId.Data) == 0 {
			return nil, errors.New("key vault returned an empty data key id")
		}
		ret = new(DataKeyId(keyId))
	}

	m.dkids.Store(altName, *ret)

	return ret, nil
}

// lookupDataKey reads the data key stored under altName in the key vault. It
// returns [ErrDataKeyNotFound] when no key exists and rejects a document
// whose _id is not a non-empty binary, so an invalid id is never cached.
func (m *encryptionClientState) lookupDataKey(ctx context.Context, altName string) (*DataKeyId, error) {
	res := m.client.GetKeyByAltName(ctx, altName)
	if err := res.Err(); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrDataKeyNotFound
		}
		return nil, err
	}

	var keyData map[string]any
	if err := res.Decode(&keyData); err != nil {
		return nil, err
	}

	idVal, ok := keyData["_id"].(bson.Binary)
	if !ok {
		return nil, errors.New("invalid _id field type in key data")
	}
	if len(idVal.Data) == 0 {
		return nil, errors.New("empty _id in key data")
	}

	return new(DataKeyId(idVal)), nil
}

// InvalidateDataKey drops the cached DataKeyId for altName so the next
// [Mongo.DataKey] call re-fetches the canonical id from the key vault.
// Call this after rotating or replacing the underlying key vault
// document out-of-band — without invalidation, the process keeps using
// the cached id for the lifetime of the [Mongo] instance and silently
// defeats key rotation.
func (m *Mongo) InvalidateDataKey(altName string) {
	m.encryption.mu.Lock()
	defer m.encryption.mu.Unlock()
	if state := m.encryption.current; state != nil {
		state.dkids.Delete(altName)
	}
}

// InvalidateAllDataKeys clears every cached DataKeyId. Useful after a
// bulk key-vault refresh or when the process suspects its cache is
// stale (e.g. after reconnecting to a different vault instance).
// Functionally equivalent to a full cache flush; the next DataKey call
// for any altName will re-fetch from the vault.
func (m *Mongo) InvalidateAllDataKeys() {
	m.encryption.mu.Lock()
	defer m.encryption.mu.Unlock()
	if state := m.encryption.current; state != nil {
		state.dkids.Clear()
	}
}

// Encrypt explicitly encrypts a value using the specified algorithm and data key.
// This provides fine-grained control over the encryption process for individual values.
//
// Parameters:
//   - ctx: Context for controlling encryption operation timeout
//   - data: Value to encrypt (must be BSON-serializable)
//   - alg: Encryption algorithm ("deterministic" or "random")
//   - altName: Alternative name of the data key to use for encryption
//
// Returns:
//   - *bson.Binary: Encrypted binary data ready for storage in MongoDB
//   - error: Error if encryption is not enabled, key not found, or encryption fails
//
// Example:
//
//	// Encrypt a credit card number with deterministic encryption (allows querying)
//	encryptedCC, err := mongo.Encrypt(ctx, "1234-5678-9012-3456", "deterministic", "cc_key")
//	if err != nil {
//		return err
//	}
//
//	// Store in document
//	doc := bson.M{"user_id": userID, "credit_card": encryptedCC}
func (m *Mongo) Encrypt(ctx context.Context, data any, alg EncryptionAlg, altName string) (*bson.Binary, error) {
	return withEncryption(ctx, &m.encryption, func(ctx context.Context, state *encryptionClientState) (*bson.Binary, error) {
		return state.encrypt(ctx, data, alg, altName)
	})
}

func (m *encryptionClientState) encrypt(ctx context.Context, data any, alg EncryptionAlg, altName string) (*bson.Binary, error) {
	dkid, err := m.dataKey(ctx, altName, true)
	if err != nil {
		return nil, err
	}
	raw, err := m.encodePlaintext(data)
	if err != nil {
		return nil, err
	}
	ciphertext, err := m.client.Encrypt(ctx, raw, mongoOptions.Encrypt().SetAlgorithm(alg.String()).SetKeyID(bson.Binary(*dkid)))
	if err != nil {
		return nil, err
	}
	return &ciphertext, nil
}

// encodePlaintext is the registry-aware equivalent of bson.MarshalValue.
// The enclosing document gives the public driver writer a place to flush one value.
func (m *encryptionClientState) encodePlaintext(data any) (bson.RawValue, error) {
	var buf bytes.Buffer
	doc, err := bson.NewDocumentWriter(&buf).WriteDocument()
	if err != nil {
		return bson.RawValue{}, err
	}
	writer, err := doc.WriteDocumentElement("v")
	if err != nil {
		return bson.RawValue{}, err
	}
	enc, err := m.registry.LookupEncoder(reflect.TypeOf(data))
	if err != nil {
		return bson.RawValue{}, err
	}
	if err = enc.EncodeValue(bson.EncodeContext{Registry: m.registry}, writer, reflect.ValueOf(data)); err != nil {
		return bson.RawValue{}, err
	}
	if err = doc.WriteDocumentEnd(); err != nil {
		return bson.RawValue{}, err
	}
	return bson.Raw(buf.Bytes()).LookupErr("v")
}

func (m *fieldEncryption) encryptDocumentField(ctx context.Context, raw bson.RawValue, algorithm, key string) (bson.Binary, error) {
	return withEncryption(ctx, m, func(ctx context.Context, state *encryptionClientState) (bson.Binary, error) {
		alg, err := EncryptionAlgFromAlias(algorithm)
		if err != nil {
			return bson.Binary{}, err
		}
		return state.client.Encrypt(ctx, raw, mongoOptions.Encrypt().SetAlgorithm(alg.String()).SetKeyAltName(key))
	})
}
