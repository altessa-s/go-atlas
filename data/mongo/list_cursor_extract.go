// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/oklog/ulid/v2"
	"go.mongodb.org/mongo-driver/v2/bson"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

// extractCursorId converts various ID types to their string representation.
// This function normalizes different ID formats into a consistent string format
// for cursor generation. It ensures the ID is non-empty and validates it's a
// MongoDB ObjectID (24 hexadecimal characters).
//
// Supported types:
//   - string: Returned as-is if non-empty and valid (must be ObjectID)
//   - bson.ObjectID: Converted to hex string representation (always valid)
//   - fmt.Stringer: Any type implementing String() method
//
// Parameters:
//   - value: ID value in any supported format
//
// Returns:
//   - string: String representation of the ID (validated as ObjectID)
//   - error: If the type is unsupported, ID is empty, or not a valid ObjectID
//
// Error conditions:
//   - Empty string ID
//   - Empty string from Stringer implementation
//   - Unsupported type (doesn't match string, bson.ObjectID, or fmt.Stringer)
//   - ID is not a valid MongoDB ObjectID (must be 24 hexadecimal characters)
//
// Example conversions:
//   - "507f1f77bcf86cd799439011" → "507f1f77bcf86cd799439011" (ObjectID)
//   - bson.ObjectID("507f1f77bcf86cd799439011") → "507f1f77bcf86cd799439011" (ObjectID)
//   - "01HQVZX3M8RFKJH9Q5W0BXYZ12" → error (ULID not supported)
//   - "550e8400-e29b-41d4-a716-446655440000" → error (UUID not supported)
func extractCursorId(value any) (string, error) {
	var id string

	switch v := value.(type) {
	case string:
		if v == "" {
			return "", fmt.Errorf("id must not be empty")
		}
		id = v
	case bson.ObjectID:
		id = v.Hex()
	default:
		if s, ok := v.(fmt.Stringer); ok {
			id = s.String()
			if id == "" {
				return "", fmt.Errorf("stringer id must not be empty")
			}
		} else {
			return "", fmt.Errorf("unsupported id type %T", value)
		}
	}

	// Validate that the ID is a valid MongoDB ObjectID
	// This prevents using ULID, UUID, or other non-ObjectID formats
	if !isValidCursorId(id) {
		return "", fmt.Errorf("id %q is not a valid MongoDB ObjectID (must be 24 hexadecimal characters)", id)
	}

	return id, nil
}

// isULID checks if a string is a valid ULID format.
// ULID format: 26 characters using Crockford's Base32 alphabet.
// First character must be 0-7 (timestamp prefix for dates up to ~10000 AD).
//
// Parameters:
//   - s: String to check
//
// Returns:
//   - true if s is a valid ULID format, false otherwise
func isULID(s string) bool {
	// ULID must be exactly ulidLength characters
	if len(s) != ulidLength {
		return false
	}

	// First character must be 0-7 (timestamp constraint)
	if s[0] < '0' || s[0] > '7' {
		return false
	}

	// Crockford's Base32 alphabet: 0-9, A-Z (excluding I, L, O, U)
	// Valid chars: 0123456789ABCDEFGHJKMNPQRSTVWXYZ
	for _, c := range s {
		if !isBase32Char(c) {
			return false
		}
	}

	return true
}

// isBase32Char checks if a rune is valid in Crockford's Base32 alphabet.
func isBase32Char(c rune) bool {
	return (c >= '0' && c <= '9') || // 0-9
		(c >= 'A' && c <= 'H') || // A-H
		c == 'J' || c == 'K' || // J, K
		(c >= 'M' && c <= 'N') || // M, N
		(c >= 'P' && c <= 'T') || // P-T
		(c >= 'V' && c <= 'Z') // V-Z
}

// parseCursorToken detects cursor format and loads cursor accordingly.
// Supports two formats:
//  1. ULID (26 chars) - Server-side storage (stateful mode), requires CursorStorage
//  2. Base64-encoded JSON - Client-side storage (stateless mode)
//
// Parameters:
//   - ctx: Context for storage operations
//   - token: Cursor token from client
//   - storage: CursorStorage instance (can be nil for stateless mode)
//   - filter: Current filter for validation
//
// Returns:
//   - *Cursor: Loaded cursor with metadata
//   - error: If parsing/loading fails
func parseCursorToken(ctx context.Context, token string, storage CursorStorage, filter bson.M) (*Cursor, error) {
	if token == "" {
		// Empty token is valid - means no cursor provided, start from beginning
		return nil, nil //nolint:nilnil // nil cursor with nil error is semantically correct here
	}

	// Detect format: ULID (26 chars) vs Base64 (variable length)
	if isULID(token) {
		// Server-side cursor format
		if storage == nil {
			return nil, coreerrs.Wrap(ErrStorageRequired, "received ULID cursor but no storage configured")
		}

		// Load metadata from storage
		metadata, err := storage.Load(ctx, token)
		if err != nil {
			return nil, coreerrs.WrapOperation(err, "load cursor from storage")
		}

		// Validate filter hasn't changed
		if validationErr := metadata.ValidateFilter(filter); validationErr != nil {
			return nil, validationErr
		}

		// Convert metadata to Cursor
		cursor, convErr := metadata.ToCursor()
		if convErr != nil {
			return nil, coreerrs.WrapOperation(convErr, "convert metadata to cursor")
		}

		return cursor, nil
	}

	// Stateless format: base64-encoded JSON
	cursor, err := ParseCursor(token)
	if err != nil {
		return nil, err
	}

	// Validate filter hasn't changed (stateless mode)
	if validationErr := cursor.ValidateFilter(filter); validationErr != nil {
		return nil, validationErr
	}

	return cursor, nil
}

// generateNextCursorToken creates the next cursor token.
// If storage is configured, stores metadata and returns ULID key (stateful mode).
// Otherwise, encodes cursor as base64 JSON (stateless mode).
//
// Parameters:
//   - ctx: Context for storage operations
//   - cursorId: MongoDB ObjectID from last item
//   - sort: Sort specification
//   - cursorIdField: Field name for cursor ID
//   - filter: Current filter
//   - storage: CursorStorage instance (can be nil)
//
// Returns:
//   - string: Cursor token for client
//   - error: If generation fails
func generateNextCursorToken(
	ctx context.Context,
	cursorId string,
	sort bson.D,
	cursorIdField string,
	filter bson.M,
	storage CursorStorage,
	sortValue any,
) (string, error) {
	if storage != nil {
		// Server-side storage mode: create metadata and store
		metadata, err := NewCursorMetadata(cursorId, sort, cursorIdField, filter, sortValue)
		if err != nil {
			return "", coreerrs.WrapOperation(err, "create cursor metadata")
		}

		// Generate ULID key
		key := ulid.Make().String()

		// Store with generated key
		if err := storage.Store(ctx, key, metadata); err != nil {
			return "", coreerrs.WrapOperation(err, "store cursor")
		}

		return key, nil
	}

	// Stateless mode: encode cursor as base64 JSON
	cursor, err := NewCursorWithMetadata(cursorId, sort, cursorIdField, filter, sortValue)
	if err != nil {
		return "", coreerrs.WrapOperation(err, "create cursor")
	}

	return cursor.String(), nil
}

// extractCursorDataFromItem extracts cursor ID and sort field value from an item.
// The sort field value is needed for compound cursor filter when sorting
// by a field other than cursor_id.
//
// This function is optimized to use reflection for direct field access on structs,
// avoiding the marshal/unmarshal overhead. Falls back to BSON encoding for non-struct types.
//
// Parameters:
//   - item: The item to extract data from
//   - cursorIdField: Field name for cursor ID
//   - sort: Sort specification to determine which field value to extract
//
// Returns:
//   - cursorId: String representation of the cursor ID
//   - sortFieldValue: Value of the sort field (nil if sorting by cursorIdField)
//   - error: If extraction fails
func extractCursorDataFromItem[T any](item T, cursorIdField string, sort bson.D) (cursorId string, sortFieldValue any, err error) {
	v := reflect.ValueOf(item)

	// Dereference pointers
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return "", nil, fmt.Errorf("nil item")
		}
		v = v.Elem()
	}

	// Fast path: use reflection for structs
	if v.Kind() == reflect.Struct {
		// Extract cursor ID field
		idValue, err := findFieldValueByBSONTag(v, cursorIdField)
		if err != nil {
			return "", nil, coreerrs.Wrapf(err, "cursor id field %q", cursorIdField)
		}

		cursorId, err = extractCursorId(idValue.Interface())
		if err != nil {
			return "", nil, coreerrs.Wrap(err, "extract cursor id")
		}

		// Extract sort field value if needed
		if len(sort) > 0 && sort[0].Key != cursorIdField {
			sortValue, err := findFieldValueByBSONTag(v, sort[0].Key)
			if err == nil {
				sortFieldValue = sortValue.Interface()
			}
			// Ignore error - sort field might not exist, which is fine
		}

		return cursorId, sortFieldValue, nil
	}

	// Slow path: fallback to BSON marshal/unmarshal for non-struct types
	return extractCursorDataViaBSON(item, cursorIdField, sort)
}

// findFieldValueByBSONTag finds a struct field by its BSON tag name.
// Searches in order: BSON tag, JSON tag (for compatibility), field name.
//
// Returns the field value if found, or error if not found.
func findFieldValueByBSONTag(v reflect.Value, tagName string) (reflect.Value, error) {
	t := v.Type()

	// Search through all fields
	for i := range t.NumField() {
		field := t.Field(i)

		// Check BSON tag first
		if bsonTag := field.Tag.Get("bson"); bsonTag != "" {
			// Parse tag (format: "name,omitempty")
			name, _, _ := strings.Cut(bsonTag, ",")
			if name == tagName {
				return v.Field(i), nil
			}
		}

		// Check JSON tag for compatibility
		if jsonTag := field.Tag.Get("json"); jsonTag != "" {
			name, _, _ := strings.Cut(jsonTag, ",")
			if name == tagName {
				return v.Field(i), nil
			}
		}

		// Check field name (case-insensitive)
		if corestrings.InternLowerString(field.Name) == corestrings.InternLowerString(tagName) {
			return v.Field(i), nil
		}
	}

	return reflect.Value{}, fmt.Errorf("field not found")
}

// extractCursorDataViaBSON is the fallback implementation using BSON marshal/unmarshal.
// Used for non-struct types (e.g., bson.M, bson.D, maps).
func extractCursorDataViaBSON[T any](item T, cursorIdField string, sort bson.D) (cursorId string, sortFieldValue any, err error) {
	itemBytes, err := bson.Marshal(item)
	if err != nil {
		return "", nil, coreerrs.WrapOperation(err, "marshal item")
	}

	var itemMap bson.M
	if err = bson.Unmarshal(itemBytes, &itemMap); err != nil {
		return "", nil, coreerrs.WrapOperation(err, "unmarshal item")
	}

	idValue, ok := itemMap[cursorIdField]
	if !ok {
		return "", nil, fmt.Errorf("cursor id field %q missing in item", cursorIdField)
	}

	cursorId, err = extractCursorId(idValue)
	if err != nil {
		return "", nil, coreerrs.Wrap(err, "extract cursor id")
	}

	if len(sort) > 0 && sort[0].Key != cursorIdField {
		sortFieldValue = itemMap[sort[0].Key]
	}

	return cursorId, sortFieldValue, nil
}
