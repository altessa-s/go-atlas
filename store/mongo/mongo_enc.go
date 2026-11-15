// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// This file contains all encryption-related functionality for MongoDB
// Client-Side Field Level Encryption (CSFLE) support.

package mongo

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/store/mongo/internal/document"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

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
	if EncryptionAlg(alias).IsValid() {
		return EncryptionAlg(alias), nil
	}

	switch {
	case strings.EqualFold(alias, "deterministic"):
		return EncryptionAlgDeterministic, nil
	case strings.EqualFold(alias, "random"):
		return EncryptionAlgRandom, nil
	}

	return "", coreerrs.Wrapf(ErrEncryptionUnknownAlg, "encryption algorithm alias: %s not supported", alias)
}

// Encryption algorithms supported by MongoDB Client-Side Field Level Encryption (CSFLE)
const (
	// EncryptionAlgDeterministic provides deterministic encryption (same input = same output)
	// Use for fields that need to be queried (indexed, searched, sorted)
	EncryptionAlgDeterministic EncryptionAlg = "AEAD_AES_256_CBC_HMAC_SHA_512-Deterministic"
	// EncryptionAlgRandom provides randomized encryption (same input = different output)
	// Use for fields that don't need to be queried but need maximum security
	EncryptionAlgRandom EncryptionAlg = "AEAD_AES_256_CBC_HMAC_SHA_512-Random"
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
	return m.config.EncryptionEnabled && m.config.KMS != nil
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
	if !m.IsEncryptionConfigured() {
		return nil, ErrEncryptionNotEnabled
	}

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

	return m.CreateDataKey(ctx, altName)
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
	if !m.IsEncryptionConfigured() {
		return nil, ErrEncryptionNotEnabled
	}

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

	if masterKey := m.config.KMS.MasterKey(); masterKey != nil {
		dataKeyOpts.SetMasterKey(masterKey)
	}

	keyId, err := m.encryptionClient.CreateDataKey(ctx, m.config.KMS.Name(), dataKeyOpts)
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
func (m *Mongo) lookupDataKey(ctx context.Context, altName string) (*DataKeyId, error) {
	res := m.encryptionClient.GetKeyByAltName(ctx, altName)
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
	m.dkids.Delete(altName)
}

// InvalidateAllDataKeys clears every cached DataKeyId. Useful after a
// bulk key-vault refresh or when the process suspects its cache is
// stale (e.g. after reconnecting to a different vault instance).
// Functionally equivalent to a full cache flush; the next DataKey call
// for any altName will re-fetch from the vault.
func (m *Mongo) InvalidateAllDataKeys() {
	m.dkids.Range(func(k, _ any) bool {
		m.dkids.Delete(k)
		return true
	})
}

// ConvertToNewDocument converts a Go struct to a BSON document for insertion operations.
// It handles field encryption using explicit encryption, removes nil/empty fields,
// and processes nested structures.
//
// Field encryption can be configured in two ways:
//  1. WithEncryptionModel() option (recommended) - configures encryption programmatically
//  2. Struct tags - `encrypted:"algorithm,keyAltName"` - encrypts the field with specified algorithm and key
//
// Other supported tags:
//   - `bson:"omitempty"` - omits empty fields from the document
//
// Supported encryption algorithms:
//   - "deterministic" - same input produces same encrypted output (allows querying)
//   - "random" - same input produces different encrypted output (maximum security)
//
// Parameters:
//   - ctx: Context for controlling encryption operations
//   - entity: Go struct or pointer to struct to convert
//
// Returns:
//   - bson.M: BSON document ready for insertion
//   - error: Error if conversion or encryption fails
//
// Example with WithEncryptionModel (recommended):
//
//	type User struct {
//		ID    string `bson:"_id"`
//		Name  string `bson:"name"`
//		Email string `bson:"email"`
//	}
//
//	model := &EncryptionModel{
//		Fields: []EncryptionField{
//			{FieldName: "Email", Algorithm: "deterministic", KeyAltName: "user_email_key"},
//		},
//	}
//
//	mongo, err := mongotools.New(
//		mongotools.WithHosts("localhost:27017"),
//		mongotools.WithKMSProvider(localKMS),
//		mongotools.WithEncryption(true),
//		mongotools.WithEncryptionModel((*User)(nil), model),
//	)
//
//	user := &User{ID: "123", Name: "John", Email: "john@example.com"}
//	doc, err := mongo.ConvertToNewDocument(ctx, user)
//	// Result: {"_id": "123", "name": "John", "email": BinaryData(encrypted)}
//
// Example with struct tags (legacy):
//
//	type User struct {
//		Email string `bson:"email" encrypted:"deterministic,user_email_key"`
//	}
func (m *Mongo) ConvertToNewDocument(ctx context.Context, entity any) (bson.M, error) {
	set, _, err := m.convertToDocument(ctx, entity, false)
	return set, err
}

// ConvertToUpdateDocument converts a Go struct to a BSON update document with $set and $unset operators.
// It handles field encryption using explicit encryption, manages nil and empty fields,
// and processes nested structures for update operations.
//
// Field behavior:
//   - Non-nil, non-empty fields -> added to $set
//   - Nil pointer fields -> added to $unset (field will be removed from document)
//   - Empty arrays/slices/maps -> added to $unset
//   - Fields with `omitonupdate` tag -> ignored completely
//   - `_id` field -> automatically excluded from $set
//
// Nested structures (pointer-to-struct fields and struct values inside maps) use
// whole-object replacement: the nested struct is emitted as a single $set
// subdocument that fully replaces the stored one. A nested nil/zero field is
// therefore simply absent from the replacement (effectively removed); no
// per-field nested $unset such as {"parent.child": nil} is generated. If you
// need dotted-path partial updates that preserve untouched nested fields already
// in the database, see PLAN_B_mongo_nested_partial_update.md at the repo root.
//
// Field encryption can be configured in two ways:
//  1. WithEncryptionModel() option (recommended) - configures encryption programmatically
//  2. Struct tags - `encrypted:"algorithm,keyAltName"` - encrypts the field
//
// Other struct tags:
//   - `bson:"fieldname,omitonupdate"` - excludes field from updates
//   - `bson:"fieldname,omitempty"` - standard MongoDB omitempty behavior
//
// Parameters:
//   - ctx: Context for controlling encryption operations
//   - entity: Go struct or pointer to struct to convert
//
// Returns:
//   - bson.M: BSON update document with $set and $unset operators
//   - error: Error if conversion or encryption fails
//
// Example with WithEncryptionModel (recommended):
//
//	type User struct {
//		ID       string  `bson:"_id"`
//		Name     string  `bson:"name"`
//		LastName *string `bson:"last_name"`
//		Email    string  `bson:"email"`
//	}
//
//	model := &EncryptionModel{
//		Fields: []EncryptionField{
//			{FieldName: "Email", Algorithm: "deterministic", KeyAltName: "user_email_key"},
//		},
//	}
//
//	mongo, err := mongotools.New(
//		mongotools.WithKMSProvider(localKMS),
//		mongotools.WithEncryption(true),
//		mongotools.WithEncryptionModel((*User)(nil), model),
//	)
//
//	user := &User{
//		ID:       "123",      // excluded from $set
//		Name:     "John",     // goes to $set
//		LastName: nil,        // goes to $unset
//		Email:    "john@example.com", // encrypted and goes to $set
//	}
//
//	doc, err := mongo.ConvertToUpdateDocument(ctx, user)
//	// Result: {"$set": {"name": "John", "email": BinaryData}, "$unset": {"last_name": nil}}
func (m *Mongo) ConvertToUpdateDocument(ctx context.Context, entity any) (bson.M, error) {
	set, unset, err := m.convertToDocument(ctx, entity, true)
	if err != nil {
		return nil, err
	}

	delete(set, "_id")

	result := bson.M{opSet: set}
	if len(unset) > 0 {
		result["$unset"] = unset
	}
	return result, nil
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
	if !m.IsEncryptionConfigured() {
		return nil, ErrEncryptionNotEnabled
	}

	dkid, err := m.DataKey(ctx, altName, true)
	if err != nil {
		return nil, err
	}
	return m.encryptField(ctx, data, alg, dkid)
}

// encryptField encrypts a field value using the specified algorithm and data key.
func (m *Mongo) encryptField(ctx context.Context, data any, alg EncryptionAlg, dke any) (*bson.Binary, error) {
	rawValueType, rawValueData, err := bson.MarshalValue(data)
	if err != nil {
		return nil, err
	}
	rawValue := bson.RawValue{Type: rawValueType, Value: rawValueData}

	encryptionOpts := mongoOptions.Encrypt().SetAlgorithm(alg.String())

	if altName, ok := dke.(string); ok {
		encryptionOpts.SetKeyAltName(altName)
	} else if dkeId, ok := dke.(*DataKeyId); ok {
		encryptionOpts.SetKeyID(bson.Binary(*dkeId))
	} else if dkeId, ok := dke.(DataKeyId); ok {
		encryptionOpts.SetKeyID(bson.Binary(dkeId))
	}

	encryptedField, err := m.encryptionClient.Encrypt(ctx, rawValue, encryptionOpts)
	if err != nil {
		return nil, err
	}

	return &encryptedField, nil
}

// convertToDocument binds database encryption to the standalone document engine.
func (m *Mongo) convertToDocument(ctx context.Context, entity any, update bool) (bson.M, bson.M, error) {
	var encrypt document.EncryptFunc
	if m.IsEncryptionConfigured() {
		encrypt = m.encryptDocumentField
	}
	return document.New(m.structParser, encrypt).Convert(ctx, entity, update)
}

func (m *Mongo) encryptDocumentField(ctx context.Context, value any, algorithm, key string) (*bson.Binary, error) {
	alg, err := EncryptionAlgFromAlias(algorithm)
	if err != nil {
		return nil, err
	}
	return m.encryptField(ctx, value, alg, key)
}
