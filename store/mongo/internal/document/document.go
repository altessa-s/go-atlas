// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package document

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-convert/reflectx"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

// ErrEncryptionNotEnabled is returned when a protected field has no encryptor.
var ErrEncryptionNotEnabled = errors.New("encryption not enabled")

// EncryptFunc encrypts one field with its schema algorithm and alternative key name.
type EncryptFunc func(context.Context, any, string, string) (*bson.Binary, error)

// Engine converts struct fields using cached metadata and an optional encryptor.
type Engine struct {
	parser  *Parser
	encrypt EncryptFunc
}

// New creates an engine. A nil encryptor rejects fields requiring encryption.
func New(parser *Parser, encrypt EncryptFunc) *Engine {
	return &Engine{parser: parser, encrypt: encrypt}
}

var (
	bsonMarshalerType      = reflect.TypeFor[bson.Marshaler]()
	bsonValueMarshalerType = reflect.TypeFor[bson.ValueMarshaler]()
)

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
	nestedType NestedStructType

	// Processing flags and state (grouped to minimize padding)
	isOmitEmpty    bool
	isOmitOnUpdate bool
	shouldEncrypt  bool
}

// Convert produces insert fields or update set/unset fields from an entity.
func (m *Engine) Convert(ctx context.Context, entity any, update bool) (bson.M, bson.M, error) {
	if entity == nil {
		return nil, nil, fmt.Errorf("document conversion failed during metadata collection: entity cannot be nil")
	}
	fields := m.parser.ParseStruct(entity).Fields
	if len(fields) == 0 {
		return nil, nil, fmt.Errorf("document conversion failed during metadata collection: entity contains no processable fields")
	}
	set, unset := make(bson.M, len(fields)), make(bson.M)
	for _, meta := range fields {
		if err := m.processFieldInto(ctx, meta, update, set, unset); err != nil {
			return nil, nil, coreerrs.Wrap(coreerrs.WrapField(err, meta.fieldName), "document conversion failed during field processing")
		}
	}
	return set, unset, nil
}

// processFieldInto writes a field directly into the shared set/unset documents.
func (m *Engine) processFieldInto(ctx context.Context, meta fieldMetadata, update bool, setDoc, unsetDoc bson.M) error {
	if update && meta.isOmitOnUpdate {
		return nil
	}
	if update && meta.fieldValue.Kind() == reflect.Pointer && meta.fieldValue.IsNil() {
		unsetDoc[meta.fieldName] = nil
		return nil
	}
	if !update && meta.isOmitEmpty && meta.fieldValue.Kind() == reflect.Pointer &&
		(meta.fieldValue.IsNil() || reflect.Indirect(meta.fieldValue).IsZero()) {
		return nil
	}
	if m.isEmptyCollection(meta.fieldValue) {
		if update {
			unsetDoc[meta.fieldName] = nil
		}
		return nil
	}
	switch {
	case meta.shouldEncrypt && meta.algorithmString != NestedEncryptionKey:
		return m.processDefaultField(ctx, meta, setDoc)
	case meta.fieldKind == reflect.Bool:
		setDoc[meta.fieldName] = meta.fieldValue.Bool()
		return nil
	case meta.fieldValue.Kind() == reflect.Slice && !m.isBytesSlice(meta.fieldValue):
		return m.processSliceField(ctx, meta, update, setDoc)
	case m.isStructPointerField(meta.fieldValue, meta.fieldType):
		return m.processStructPointerField(ctx, meta, update, setDoc)
	case meta.fieldValue.Kind() == reflect.Map:
		return m.processMapField(ctx, meta, update, setDoc)
	default:
		return m.processDefaultField(ctx, meta, setDoc)
	}
}

// isEmptyCollection checks if a field represents an empty collection (slice, map, or array).
func (m *Engine) isEmptyCollection(fieldValue reflect.Value) bool {
	return (fieldValue.Kind() == reflect.Slice ||
		fieldValue.Kind() == reflect.Map ||
		fieldValue.Kind() == reflect.Array) && fieldValue.Len() == 0
}

// isBytesSlice checks if a field is a []byte or []uint8 (which are identical types in Go).
// Both []byte and []uint8 should be treated as BSON Binary values, not as arrays of elements.
// Note: In Go, byte is an alias for uint8, so reflect cannot distinguish between them.
// BSON encoder treats both types identically as Binary data.
func (m *Engine) isBytesSlice(fieldValue reflect.Value) bool {
	return fieldValue.Kind() == reflect.Slice && fieldValue.Type().Elem().Kind() == reflect.Uint8
}

// isStructPointerField checks if a field is a pointer to a struct that should
// be recursed into for document processing.
func (m *Engine) isStructPointerField(fieldValue reflect.Value, fieldType reflect.StructField) bool {
	if fieldValue.Type().Kind() != reflect.Pointer || fieldValue.IsNil() {
		return false
	}
	elem := reflectx.IndirectType(fieldType.Type)
	return elem.Kind() == reflect.Struct && recurseIntoStruct(elem)
}

// structRecursionCache memoizes recurseIntoStruct decisions. The set of struct
// types encountered during document conversion is finite, so an unbounded
// sync.Map is sufficient and avoids the per-element reflection scan on hot
// slice/map walks.
var structRecursionCache sync.Map // map[reflect.Type]bool

// recurseIntoStruct reports whether a struct of type t should be walked
// field-by-field during document conversion. A struct is walked only when it
// exposes exported fields AND does not define its own BSON serialization:
//   - Opaque structs such as time.Time carry only unexported fields and the BSON
//     driver serializes them as scalar values; recursing into them yields
//     "entity contains no processable fields".
//   - Types implementing bson.Marshaler/bson.ValueMarshaler serialize themselves
//     into a single value, so walking their exported fields would discard the
//     custom encoding and produce a wrong document.
//
// Both cases are treated as scalar leaves. Decisions are memoized because
// slice/map walks query this once per element.
func recurseIntoStruct(t reflect.Type) bool {
	if cached, ok := structRecursionCache.Load(t); ok {
		walk, _ := cached.(bool)
		return walk
	}
	walk := hasExportedField(t) && !implementsBSONMarshaler(t)
	structRecursionCache.Store(t, walk)
	return walk
}

// hasExportedField reports whether t has at least one exported field.
func hasExportedField(t reflect.Type) bool {
	for field := range t.Fields() {
		if field.IsExported() {
			return true
		}
	}
	return false
}

// implementsBSONMarshaler reports whether t or *t implements a custom BSON
// marshaling interface (bson.Marshaler or bson.ValueMarshaler). The pointer
// type's method set is a superset of the value type's, so checking *t covers
// both value- and pointer-receiver implementations.
func implementsBSONMarshaler(t reflect.Type) bool {
	ptr := reflect.PointerTo(t)
	return ptr.Implements(bsonMarshalerType) || ptr.Implements(bsonValueMarshalerType)
}

// processSliceField handles the conversion of slice fields.
func (m *Engine) processSliceField(
	ctx context.Context,
	meta fieldMetadata,
	update bool,
	setDoc bson.M,
) error {
	itemsSetDoc := bson.A{}

	for i := range meta.fieldValue.Len() {
		// Scalar-leaf structs (opaque like time.Time, or custom BSON marshalers)
		// are serialized by the driver as a single value; recursing into them
		// would fail or discard the custom encoding. See recurseIntoStruct.
		elem := reflect.Indirect(meta.fieldValue.Index(i))
		if elem.Kind() != reflect.Struct || !recurseIntoStruct(elem.Type()) {
			itemsSetDoc = append(itemsSetDoc, meta.fieldValue.Index(i).Interface())
			continue
		}

		chSetDoc, _, err := m.Convert(ctx, meta.fieldValue.Index(i).Interface(),
			update)
		if err != nil {
			return err
		}

		itemsSetDoc = append(itemsSetDoc, chSetDoc)
	}

	setDoc[meta.fieldName] = itemsSetDoc

	return nil
}

// processStructPointerField handles the conversion of pointer-to-struct fields.
//
// Nested structs use whole-object replacement: the nested $set subdocument is
// written as a single value under the parent field, fully replacing the stored
// subdocument. Nested nil/zero fields are absent from that subdocument and are
// therefore already removed by the replacement, so the recursion's nested
// $unset map is intentionally discarded — emitting it as {parent: {child: ...}}
// would be an invalid MongoDB $unset (it would drop the whole parent). See
// ConvertToUpdateDocument for the documented contract.
func (m *Engine) processStructPointerField(ctx context.Context, meta fieldMetadata, update bool,
	setDoc bson.M) error {
	chSetDoc, _, err := m.Convert(ctx, meta.fieldValue.Interface(),
		update)
	if err != nil {
		return err
	}

	setDoc[meta.fieldName] = chSetDoc
	return nil
}

// processMapField handles the conversion of map fields.
func (m *Engine) processMapField(ctx context.Context, meta fieldMetadata, update bool,
	setDoc bson.M) error {
	mm := bson.M{}
	mapiter := meta.fieldValue.MapRange()
	for mapiter.Next() {
		if mapiter.Key().Kind() != reflect.String {
			return fmt.Errorf("%s: map key must be string", meta.fieldName)
		}
		mapKey := mapiter.Key().String()
		// Reject MongoDB operator-like keys ($-prefixed) before they reach the
		// BSON document. A map[string]any field carrying a key such as "$set"
		// would otherwise be embedded verbatim and could be interpreted as an
		// update operator, mutating fields outside the intended scope.
		if strings.HasPrefix(mapKey, "$") {
			return fmt.Errorf("%s: map key %q must not start with %q (reserved for MongoDB operators)", meta.fieldName, mapKey, "$")
		}
		// Same scalar-leaf guard as processSliceField: time.Time and custom BSON
		// marshalers are scalars to the driver, not documents to recurse into.
		mapElem := reflect.Indirect(mapiter.Value())
		if mapElem.Kind() == reflect.Struct && recurseIntoStruct(mapElem.Type()) {
			// Whole-object replacement, as in processStructPointerField: the map
			// value's nested $set subdocument fully replaces the stored one, so
			// the recursion's nested $unset map is intentionally discarded.
			chSetDoc, _, err := m.Convert(ctx, mapiter.Value().Interface(), update)
			if err != nil {
				return err
			}
			mm[mapKey] = chSetDoc
			continue
		}
		mm[mapKey] = mapiter.Value().Interface()
	}
	setDoc[meta.fieldName] = mm
	return nil
}

// processDefaultField handles the default case for field processing, including encryption.
func (m *Engine) processDefaultField(ctx context.Context, meta fieldMetadata, setDoc bson.M) error {
	setDoc[meta.fieldName] = meta.fieldValue.Interface()

	// Zero values are encrypted too: storing "", 0 or false in clear would
	// leak the value of an encrypted field. Only a nil pointer or interface,
	// written as BSON null, carries no value to protect.
	if !isNilValue(meta.fieldValue) && meta.shouldEncrypt && meta.algorithmString != "" && meta.keyAltName != "" {
		// Fail closed: a field marked for encryption must never be written
		// when encryption is not configured. Without this guard the code
		// would call the (possibly nil) encryption client directly, bypassing
		// the IsEncryptionConfigured check that Encrypt enforces.
		if m.encrypt == nil {
			return coreerrs.Wrapf(ErrEncryptionNotEnabled,
				"field %q is marked for encryption but encryption is not configured", meta.fieldName)
		}

		encryptedField, err := m.encrypt(ctx, meta.fieldValue.Interface(), meta.algorithmString, meta.keyAltName)
		if err != nil {
			return err
		}

		setDoc[meta.fieldName] = encryptedField
	}
	return nil
}

// isNilValue reports whether v is a nil pointer or interface.
func isNilValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		return v.IsNil()
	default:
		return false
	}
}
