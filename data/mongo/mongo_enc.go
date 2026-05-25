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
	"reflect"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/data/mongo/kms"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Ensure kms package is recognized as used
var _ kms.Provider

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

// Field processing constants
const (
	// NestedEncryptionKey is used in encryption tag values to indicate nested field encryption
	NestedEncryptionKey = "nested"
)

// NestedStructType represents the type of nested structure in a field
type NestedStructType int

// Constants for nested structure types
const (
	// NoNesting indicates the field does not contain a nested struct.
	NoNesting NestedStructType = iota
	// PointerStruct indicates the field is a pointer to a struct.
	PointerStruct
	// SliceStruct indicates the field is a slice of structs.
	SliceStruct
	// MapStruct indicates the field is a map with struct values.
	MapStruct
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

	dataKeyExists := true
	res := m.encryptionClient.GetKeyByAltName(ctx, altName)
	if err := res.Err(); err != nil {
		if !errors.Is(err, mongo.ErrNoDocuments) {
			return nil, err
		}
		dataKeyExists = false
	}

	if dataKeyExists {
		var keyData map[string]any
		if err := res.Decode(&keyData); err != nil {
			return nil, err
		}

		if idVal, ok := keyData["_id"].(bson.Binary); ok {
			ret := DataKeyId(idVal)
			m.dkids.Store(altName, ret)
			return &ret, nil
		}
		return nil, errors.New("invalid _id field type in key data")
	}

	if !create {
		return nil, ErrDataKeyNotFound
	}

	return m.CreateDataKey(ctx, altName)
}

// CreateDataKey creates a new data encryption key with the specified alternative name.
// If a key with the same alternative name already exists, it returns the existing key.
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

	res := m.encryptionClient.GetKeyByAltName(ctx, altName)
	if err := res.Err(); err != nil {
		if !errors.Is(err, mongo.ErrNoDocuments) {
			return nil, err
		}
	} else {
		var keyData map[string]any
		if err = res.Decode(&keyData); err != nil {
			return nil, err
		}

		if idVal, ok := keyData["_id"].(bson.Binary); ok {
			ret := DataKeyId(idVal)
			return &ret, nil
		}
		return nil, errors.New("invalid _id field type in key data")
	}

	// Create a new data key for the encrypted field.
	dataKeyOpts := mongoOptions.DataKey().
		SetKeyAltNames([]string{altName})

	if masterKey := m.config.KMS.MasterKey(); masterKey != nil {
		dataKeyOpts.SetMasterKey(masterKey)
	}

	keyId, err := m.encryptionClient.CreateDataKey(ctx, m.config.KMS.Name(), dataKeyOpts)
	if err != nil {
		serr, ok := coreerrs.AsType[mongo.ServerError](err)
		if !ok || !serr.HasErrorCode(MongoErrorCodeDuplicateKey) {
			return nil, err
		}
	}

	ret := DataKeyId(keyId)
	m.dkids.Store(altName, ret)

	return &ret, nil
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
	set, _, err := m.convertToDocument(ctx, entity, false, m.IsEncryptionConfigured())
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
	set, unset, err := m.convertToDocument(ctx, entity, true, m.IsEncryptionConfigured())
	if err != nil {
		return nil, err
	}

	delete(set, "_id")

	result := bson.M{"$set": set}
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

// fieldMetadata represents parsed metadata for a struct field during document conversion.
// This separates metadata collection from business logic processing.
type fieldMetadata struct {
	// Field reflection information
	fieldType  reflect.StructField
	fieldIndex []int
	fieldValue reflect.Value
	fieldName  string
	fieldKind  reflect.Kind

	// Encryption configuration
	algorithmString string
	keyAltName      string

	// Nested structure information
	nestedType     NestedStructType
	nestedMetadata []fieldMetadata // Metadata for nested struct fields

	// Processing flags and state (grouped to minimize padding)
	isOmitEmpty    bool
	isOmitOnUpdate bool
	shouldEncrypt  bool
	hasNestedData  bool // Indicates if nested parsing was performed
}

// collectFieldsMetadata extracts and parses metadata for all struct fields.
// Uses the Mongo instance's shared parser whose sharded cache persists across calls,
// avoiding per-call parser creation and making the cache effective.
func (m *Mongo) collectFieldsMetadata(entity any, _, _ bool) []fieldMetadata {
	structMeta := m.structParser.ParseStruct(entity)
	return structMeta.Fields
}

// convertToDocument converts a Go struct to BSON documents for MongoDB operations.
// It handles field processing, encryption, and document structure generation.
// This function has been refactored to separate metadata collection from business logic processing.
func (m *Mongo) convertToDocument(ctx context.Context, entity any, update, encrypt bool) (bson.M, bson.M, error) {
	processor := m.newDocumentProcessor()
	return processor.convertEntityToDocuments(ctx, entity, update, encrypt)
}

// processFieldInto processes a single field, writing directly to shared set/unset maps.
// This avoids per-field map allocation and subsequent merge overhead.
func (m *Mongo) processFieldInto(ctx context.Context, meta fieldMetadata, update bool, setDoc, unsetDoc bson.M) error {
	processor := &documentFieldProcessor{
		mongo:  m,
		ctx:    ctx,
		meta:   meta,
		update: update,
		result: fieldProcessingResult{
			setDoc:   setDoc,
			unsetDoc: unsetDoc,
		},
	}

	if result := processor.applyPreProcessingFilters(); result.skip {
		return nil
	}

	_, _, err := processor.processFieldDirect()
	return err
}

// isEmptyCollection checks if a field represents an empty collection (slice, map, or array).
func (m *Mongo) isEmptyCollection(fieldValue reflect.Value) bool {
	return (fieldValue.Kind() == reflect.Slice ||
		fieldValue.Kind() == reflect.Map ||
		fieldValue.Kind() == reflect.Array) && fieldValue.Len() == 0
}

// isBytesSlice checks if a field is a []byte or []uint8 (which are identical types in Go).
// Both []byte and []uint8 should be treated as BSON Binary values, not as arrays of elements.
// Note: In Go, byte is an alias for uint8, so reflect cannot distinguish between them.
// BSON encoder treats both types identically as Binary data.
func (m *Mongo) isBytesSlice(fieldValue reflect.Value) bool {
	return fieldValue.Kind() == reflect.Slice && fieldValue.Type().Elem().Kind() == reflect.Uint8
}

// isSliceField checks if a field is a non-empty slice that should be processed.
func (m *Mongo) isSliceField(fieldValue reflect.Value, fieldType reflect.StructField, update bool) bool {
	return fieldValue.Kind() == reflect.Slice && fieldValue.Len() > 0 &&
		(!update || !m.isOmitOnUpdate(fieldType))
}

// isStructPointerField checks if a field is a pointer to a struct.
func (m *Mongo) isStructPointerField(fieldValue reflect.Value, fieldType reflect.StructField) bool {
	return fieldValue.Type().Kind() == reflect.Pointer && !fieldValue.IsNil() &&
		indirectType(fieldType.Type).Kind() == reflect.Struct
}

// isMapField checks if a field is a non-empty map that should be processed.
func (m *Mongo) isMapField(fieldValue reflect.Value, fieldType reflect.StructField, update bool) bool {
	return fieldValue.Kind() == reflect.Map && fieldValue.Len() > 0 &&
		(!update || !m.isOmitOnUpdate(fieldType))
}

// processSliceField handles the conversion of slice fields.
func (m *Mongo) processSliceField(
	ctx context.Context,
	meta fieldMetadata,
	update bool,
	setDoc bson.M,
	unsetDoc bson.M,
) (bson.M, bson.M, error) {
	itemsSetDoc := bson.A{}

	for i := range meta.fieldValue.Len() {
		if reflect.Indirect(meta.fieldValue.Index(i)).Kind() != reflect.Struct {
			itemsSetDoc = append(itemsSetDoc, meta.fieldValue.Index(i).Interface())
			continue
		}

		chSetDoc, _, err := m.convertToDocument(ctx, meta.fieldValue.Index(i).Interface(),
			update, meta.shouldEncrypt && meta.algorithmString == NestedEncryptionKey)
		if err != nil {
			return setDoc, unsetDoc, err
		}

		itemsSetDoc = append(itemsSetDoc, chSetDoc)
	}

	setDoc[meta.fieldName] = itemsSetDoc

	return setDoc, unsetDoc, nil
}

// processStructPointerField handles the conversion of pointer-to-struct fields.
func (m *Mongo) processStructPointerField(ctx context.Context, meta fieldMetadata, update bool,
	setDoc, unsetDoc bson.M) (bson.M, bson.M, error) {
	chSetDoc, chUnSetDoc, err := m.convertToDocument(ctx, meta.fieldValue.Interface(),
		update, meta.shouldEncrypt && meta.algorithmString == NestedEncryptionKey)
	if err != nil {
		return setDoc, unsetDoc, err
	}

	setDoc[meta.fieldName] = chSetDoc
	if len(chUnSetDoc) > 0 && len(setDoc) == 0 {
		unsetDoc[meta.fieldName] = chUnSetDoc
	}
	return setDoc, unsetDoc, nil
}

// processMapField handles the conversion of map fields.
func (m *Mongo) processMapField(ctx context.Context, meta fieldMetadata, update bool,
	setDoc, unsetDoc bson.M) (bson.M, bson.M, error) {
	mm := bson.M{}
	mapiter := meta.fieldValue.MapRange()
	for mapiter.Next() {
		if mapiter.Key().Kind() != reflect.String {
			return setDoc, unsetDoc, fmt.Errorf("%s: map key must be string", meta.fieldName)
		}
		mapKey := mapiter.Key().String()
		if reflect.Indirect(mapiter.Value()).Kind() == reflect.Struct {
			chSetDoc, chUnSetDoc, err := m.convertToDocument(ctx, mapiter.Value().Interface(), update, false)
			if err != nil {
				return setDoc, unsetDoc, err
			}
			mm[mapKey] = chSetDoc
			if len(chUnSetDoc) > 0 && len(setDoc) == 0 {
				mm[mapKey] = chUnSetDoc
			}
			continue
		}
		mm[mapKey] = mapiter.Value().Interface()
	}
	setDoc[meta.fieldName] = mm
	return setDoc, unsetDoc, nil
}

// processDefaultField handles the default case for field processing, including encryption.
func (m *Mongo) processDefaultField(ctx context.Context, meta fieldMetadata, setDoc bson.M) (bson.M, bson.M, error) {
	unsetDoc := bson.M{}
	setDoc[meta.fieldName] = meta.fieldValue.Interface()

	if !meta.fieldValue.IsZero() && meta.shouldEncrypt && meta.algorithmString != "" && meta.keyAltName != "" {
		alg, err := EncryptionAlgFromAlias(meta.algorithmString)
		if alg == "" {
			return setDoc, unsetDoc, err
		}

		encryptedField, err := m.encryptField(ctx, meta.fieldValue.Interface(), alg, meta.keyAltName)
		if err != nil {
			return setDoc, unsetDoc, err
		}

		setDoc[meta.fieldName] = encryptedField
	}
	return setDoc, unsetDoc, nil
}

// isOmitOnUpdate checks if a field should be omitted on update operations based on the configured BSON tag.
// Note: On the hot path this is redundant (applyPreProcessingFilters already checks meta.isOmitOnUpdate),
// but kept for safety in case isSliceField/isMapField are called from other contexts.
func (m *Mongo) isOmitOnUpdate(field reflect.StructField) bool {
	tag := field.Tag.Get(m.config.BSONTagName)
	return strings.Contains(tag, "omitonupdate")
}
