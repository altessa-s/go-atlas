// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"cmp"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"

	corehash "github.com/altessa-s/go-atlas/core/encoding/hash"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// Validation constants for cursor ID formats (internal implementation details)
const (
	objectIDLength = 24 // Length of a MongoDB ObjectID hex string
)

// Hex character lookup table for fast validation.
// Using [256]bool array provides O(1) lookup without hash overhead.
// Initialized once at package init time.
var hexChars [256]bool

func init() {
	// Initialize hex character lookup table
	for c := '0'; c <= '9'; c++ {
		hexChars[c] = true
	}
	for c := 'a'; c <= 'f'; c++ {
		hexChars[c] = true
	}
	for c := 'A'; c <= 'F'; c++ {
		hexChars[c] = true
	}
}

// ErrInvalidCursor is returned by [ParseCursor] when the encoded cursor
// string is empty, malformed, or contains an invalid cursor ID.
var ErrInvalidCursor = errors.New("invalid cursor")

// ErrCursorChecksumMismatch is returned by [Cursor.ValidateChecksum] when the
// cursor's checksum does not match the expected value, indicating that pagination
// parameters (sort order or cursor ID field) have changed between requests.
// Clients must restart pagination with the new parameters.
var ErrCursorChecksumMismatch = errors.New("cursor checksum mismatch")

// Cursor represents a position in a paginated dataset for cursor-based pagination.
// Uses MongoDB ObjectID for efficient and stable pagination.
// Automatically serializes to/from base64-encoded JSON for API transmission.
//
// Cursor-based pagination provides O(1) performance regardless of dataset size,
// unlike offset-based pagination which degrades to O(n) for large offsets.
//
// The cursor encodes the position using CursorId (MongoDB ObjectID), which contains:
//   - Timestamp: First 4 bytes (Unix timestamp in seconds)
//   - Machine ID: 3 bytes
//   - Process ID: 2 bytes
//   - Counter: 3 bytes
//   - Lexicographic sortability: Guarantees correct pagination order
//
// IMPORTANT: CursorId MUST be a valid MongoDB ObjectID (24 hexadecimal characters).
//
// Example usage:
//
//	cursor := mongotools.NewCursor("507f1f77bcf86cd799439011") // ObjectID
//	encoded := cursor.String() // Returns base64-encoded string
//	decoded, err := mongotools.ParseCursor(encoded)
type Cursor struct {
	// CursorId is the MongoDB ObjectID used for pagination.
	// Must be a valid ObjectID (24 hexadecimal characters).
	// ObjectID contains timestamp, ensuring lexicographic sortability.
	// This field corresponds to the _id field in your MongoDB collection.
	CursorId string `json:"c"`

	// Sort is the MongoDB sort specification (e.g., bson.D{{"created_at", -1}}).
	// This ensures the cursor can only be used with the same sort order between requests.
	// Stored as base64-encoded BSON for compact representation.
	// If empty, defaults to DefaultListSort (created_at descending).
	// Uses omitempty to reduce cursor size when using default sort.
	Sort string `json:"s,omitempty"`

	// CursorIdField is the MongoDB field name containing the cursor ID (e.g., "cursor_id", "_id").
	// This ensures the cursor uses the same ID field between requests.
	// If empty, defaults to DefaultCursorIdField ("cursor_id").
	// Uses omitempty to reduce cursor size when using default value.
	CursorIdField string `json:"cf,omitempty"`

	// Checksum is a hash of the cursor's critical parameters (sort specification, cursor ID field).
	// Used to validate that pagination parameters haven't changed between requests.
	Checksum string `json:"cs"`

	// FilterHash is a SHA-256 hash of the filter used in the query.
	// Used to validate that the filter hasn't changed between pagination requests.
	// This prevents clients from changing query filters mid-pagination.
	// Always computed for consistency and security.
	FilterHash string `json:"fh"`

	// SortValue stores the base64-encoded BSON value of the primary sort field.
	// Stored as string to preserve MongoDB type precision (Decimal128, Date, ObjectID).
	// Required for compound cursor filter when sorting by a field other than cursor_id.
	// Without this, pagination may return duplicates or skip items.
	// Empty string if sorting by cursorIdField or value is nil.
	SortValue string `json:"sv,omitempty"`
}

// NewCursor creates a new Cursor instance with validation.
// The cursorId must be a valid MongoDB ObjectID.
//
// Parameters:
//   - cursorId: MongoDB ObjectID as a 24-character hexadecimal string
//
// Returns an error if cursorId is invalid.
//
// Example:
//
//	cursor, err := mongotools.NewCursor("507f1f77bcf86cd799439011")
//	if err != nil {
//	    // Handle invalid cursor ID
//	}
func NewCursor(cursorId string) (*Cursor, error) {
	// Validate cursor ID format
	if !isValidCursorId(cursorId) {
		return nil, fmt.Errorf("invalid cursor id: must be a MongoDB ObjectID (24 hex characters)")
	}

	return &Cursor{
		CursorId: cursorId,
	}, nil
}

// MustNewCursor creates a new Cursor instance, panicking on validation error.
// Use this only when you are certain the cursorId is valid.
//
// Parameters:
//   - cursorId: MongoDB ObjectID as a 24-character hexadecimal string
//
// Panics if cursorId is not a valid MongoDB ObjectID.
//
// Example:
//
//	cursor := mongotools.MustNewCursor("507f1f77bcf86cd799439011")
func MustNewCursor(cursorId string) *Cursor {
	cursor, err := NewCursor(cursorId)
	if err != nil {
		panic(err)
	}
	return cursor
}

// String encodes the cursor to base64 string for API transmission.
// Returns an empty string if the cursor is nil.
//
// The encoding process:
//  1. Marshal cursor to JSON
//  2. Base64 encode the JSON string
//
// Example:
//
//	cursor := mongotools.MustNewCursor("507f1f77bcf86cd799439011")
//	encoded := cursor.String()
func (c *Cursor) String() string {
	if c == nil {
		return ""
	}

	// JSON encode using anonymous struct to avoid infinite recursion
	// with MarshalJSON method
	type cursorAlias Cursor
	jsonBytes, err := json.Marshal((*cursorAlias)(c))
	if err != nil {
		// Defensive: should never happen with a simple struct, but don't crash the process.
		return ""
	}

	// Base64 encode
	return base64.URLEncoding.EncodeToString(jsonBytes)
}

// MarshalJSON implements json.Marshaler interface.
// Encodes cursor as a base64 string in JSON.
//
// This allows the cursor to be transparently serialized as a simple string
// in JSON responses while maintaining its internal structure.
//
// Example:
//
//	cursor := mongotools.MustNewCursor("507f1f77bcf86cd799439011")
//	json.Marshal(cursor)
func (c *Cursor) MarshalJSON() ([]byte, error) {
	if c == nil {
		return []byte("null"), nil
	}
	return json.Marshal(c.String())
}

// UnmarshalJSON implements json.Unmarshaler interface.
// Decodes cursor from base64 string in JSON.
//
// This allows the cursor to be transparently deserialized from a simple string
// in JSON requests while reconstructing its internal structure.
//
// Returns error if:
//   - JSON unmarshaling fails
//   - Base64 decoding fails
//   - Cursor validation fails (invalid or missing cursor_id)
//
// Example:
//
//	var cursor mongotools.Cursor
//	json.Unmarshal([]byte(`"eyJjIjoiNTA3ZjFmNzdiY2Y4NmNkNzk5NDM5MDExIn0="`), &cursor)
func (c *Cursor) UnmarshalJSON(data []byte) error {
	var encoded string
	if err := json.Unmarshal(data, &encoded); err != nil {
		return err
	}

	parsed, err := ParseCursor(encoded)
	if err != nil {
		return err
	}

	if parsed == nil {
		return fmt.Errorf("parsed cursor is nil")
	}

	*c = *parsed
	return nil
}

// ParseCursor decodes a base64-encoded cursor string.
// Returns error if the string is empty or invalid.
//
// Validation rules:
//   - Encoded string must not be empty
//   - CursorId must not be empty
//   - CursorId must be a valid MongoDB ObjectID (24 hexadecimal characters)
//
// Example:
//
//	cursor, err := mongotools.ParseCursor("eyJjIjoiNTA3ZjFmNzdiY2Y4NmNkNzk5NDM5MDExIn0=")
//	if err != nil {
//	    // Handle error
//	}
func ParseCursor(encoded string) (*Cursor, error) {
	if encoded == "" {
		return nil, coreerrs.Wrap(ErrInvalidCursor, "empty string")
	}

	// Base64 decode
	jsonBytes, err := base64.URLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, coreerrs.Wrapf(ErrInvalidCursor, "%v", err)
	}

	// JSON decode using alias to avoid calling UnmarshalJSON
	type cursorAlias Cursor
	var c cursorAlias
	if err = json.Unmarshal(jsonBytes, &c); err != nil {
		return nil, coreerrs.Wrapf(ErrInvalidCursor, "%v", err)
	}

	// Validation
	if c.CursorId == "" {
		return nil, coreerrs.Wrap(ErrInvalidCursor, "cursor_id is required")
	}

	// Validate cursor ID format
	if !isValidCursorId(c.CursorId) {
		return nil, coreerrs.Wrap(ErrInvalidCursor, "invalid cursor: cursor_id must be a MongoDB ObjectID (24 hex characters)")
	}

	return (*Cursor)(&c), nil
}

// IsZero returns true if the cursor is nil or has zero values.
//
// A cursor is considered zero if:
//   - It is nil, OR
//   - CursorId is empty
//
// Example:
//
//	cursor := &mongotools.Cursor{}
//	if cursor.IsZero() {
//	    // Cursor is empty
//	}
func (c *Cursor) IsZero() bool {
	return c == nil || c.CursorId == ""
}

// isValidCursorId validates that the cursor ID is a valid MongoDB ObjectID.
// Only supports ObjectID format (24 hexadecimal characters).
//
// Parameters:
//   - id: The cursor ID string to validate
//
// Returns:
//   - true if the ID is a valid ObjectID
//   - false otherwise
//
// Example: 507f1f77bcf86cd799439011
func isValidCursorId(id string) bool {
	if len(id) != objectIDLength {
		return false
	}

	// Use array lookup table for fast hex validation
	// Array access is faster than map lookup (no hash computation)
	for i := range objectIDLength {
		if !hexChars[id[i]] {
			return false
		}
	}

	return true
}

// encodeSortToString encodes a bson.D sort specification to a base64-encoded BSON string.
// This provides a compact and accurate representation of the sort order in the cursor.
//
// Parameters:
//   - sort: MongoDB sort specification as bson.D
//
// Returns:
//   - Base64-encoded BSON string
//   - error if BSON marshaling fails
//
// Example:
//
//	sort := bson.D{{"created_at", -1}, {"_id", -1}}
//	encoded, err := encodeSortToString(sort)
func encodeSortToString(sort bson.D) (string, error) {
	if len(sort) == 0 {
		return "", fmt.Errorf("sort must not be empty")
	}

	// Marshal to BSON
	bsonBytes, err := bson.Marshal(sort)
	if err != nil {
		return "", coreerrs.WrapOperation(err, "marshal sort")
	}

	// Encode to base64
	return base64.StdEncoding.EncodeToString(bsonBytes), nil
}

// decodeSortFromString decodes a base64-encoded BSON string back to bson.D.
//
// Parameters:
//   - encoded: Base64-encoded BSON string
//
// Returns:
//   - bson.D sort specification
//   - error if decoding or unmarshaling fails
//
// Example:
//
//	sort, err := decodeSortFromString(encoded)
func decodeSortFromString(encoded string) (bson.D, error) {
	if encoded == "" {
		return nil, fmt.Errorf("encoded sort must not be empty")
	}

	// Decode from base64
	bsonBytes, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "decode sort")
	}

	// Unmarshal from BSON
	var sort bson.D
	if err = bson.Unmarshal(bsonBytes, &sort); err != nil {
		return nil, coreerrs.WrapOperation(err, "unmarshal sort")
	}

	if len(sort) == 0 {
		return nil, fmt.Errorf("decoded sort is empty")
	}

	return sort, nil
}

// encodeSortValue encodes a sort field value to a base64-encoded BSON string.
// This preserves MongoDB type precision (Decimal128, ObjectID, Date, etc.) which
// would be lost with JSON encoding.
//
// Parameters:
//   - value: The sort field value to encode (can be any MongoDB-compatible type)
//
// Returns:
//   - Base64-encoded BSON string (empty string if value is nil)
//   - error if BSON marshaling fails
//
// Example:
//
//	encoded, err := encodeSortValue(primitive.DateTime(1234567890000))
func encodeSortValue(value any) (string, error) {
	if value == nil {
		return "", nil
	}

	// Wrap value in ordered document to marshal it (bson.D avoids map allocation).
	bsonBytes, err := bson.Marshal(bson.D{{Key: "v", Value: value}})
	if err != nil {
		return "", coreerrs.WrapOperation(err, "marshal sort value")
	}

	return base64.StdEncoding.EncodeToString(bsonBytes), nil
}

// decodeSortValue decodes a base64-encoded BSON string back to its original value.
// Returns the decoded value with its original MongoDB type preserved.
//
// Parameters:
//   - encoded: Base64-encoded BSON string
//
// Returns:
//   - Decoded value with original type (nil if encoded is empty)
//   - error if decoding or unmarshaling fails
//
// Example:
//
//	value, err := decodeSortValue(encoded)
func decodeSortValue(encoded string) (any, error) {
	if encoded == "" {
		// Empty string is valid - means no sort value stored
		return nil, nil //nolint:nilnil
	}

	// Decode from base64
	bsonBytes, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "decode sort value")
	}

	// Unmarshal BSON document
	var doc bson.M
	if err := bson.Unmarshal(bsonBytes, &doc); err != nil {
		return nil, coreerrs.WrapOperation(err, "unmarshal sort value")
	}

	// Extract value from wrapper document
	value, ok := doc["v"]
	if !ok {
		return nil, fmt.Errorf("sort value document missing 'v' field")
	}

	return value, nil
}

// isSortEqual compares two bson.D sort specifications for equality.
// Returns true if both sorts have the same fields in the same order with the same values.
func isSortEqual(a, b bson.D) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Key != b[i].Key || a[i].Value != b[i].Value {
			return false
		}
	}
	return true
}

// computeCursorChecksum calculates a SHA-256 checksum of the cursor's critical parameters.
// This checksum is used to validate that pagination parameters haven't changed between requests.
//
// The checksum is computed from:
//   - Sort: Ensures the same sort specification is used (encoded as base64 BSON)
//   - CursorIdField: Ensures the same ID field is used
//
// Parameters:
//   - sort: MongoDB sort specification as bson.D
//   - cursorIdField: MongoDB field name for cursor ID (e.g., "cursor_id", "_id")
//
// Returns:
//   - Hex-encoded SHA-256 hash (64 characters)
//   - error if sort encoding fails
//
// Example:
//
//	checksum, err := computeCursorChecksum(bson.D{{"created_at", -1}}, "cursor_id")
func computeCursorChecksum(sort bson.D, cursorIdField string) (string, error) {
	// Encode sort to base64-encoded BSON for deterministic representation
	encodedSort, err := encodeSortToString(sort)
	if err != nil {
		return "", coreerrs.WrapOperation(err, "encode sort for checksum")
	}

	// Create deterministic string from cursor parameters.
	// String concat is faster than fmt.Sprintf for simple concatenation.
	return corehash.SHA256HexString(encodedSort + "|" + cursorIdField), nil
}

// NewCursorWithMetadata creates a new Cursor instance with pagination metadata and validation.
// This is the recommended way to create cursors for use with ListCursor pagination.
//
// The cursor includes metadata (sort specification, cursor ID field, filter hash) and a checksum
// to ensure pagination parameters remain consistent between requests. The filter hash is always
// computed to prevent filter changes mid-pagination, even if the filter is empty.
//
// Parameters:
//   - cursorId: MongoDB ObjectID as a 24-character hexadecimal string
//   - sort: MongoDB sort specification as bson.D (e.g., bson.D{{"created_at", -1}})
//   - cursorIdField: MongoDB field name for cursor ID (e.g., "_id")
//   - filter: MongoDB filter for validation (can be nil/empty, hash will still be computed)
//   - sortValue: Value of the sort field from the last item (nil if sorting by cursorIdField)
//
// Returns an error if:
//   - cursorId is not a valid MongoDB ObjectID (24 hexadecimal characters)
//   - sort is empty
//   - cursorIdField is empty
//   - sort encoding fails
//   - checksum computation fails
//
// Example:
//
//	cursor, err := mongotools.NewCursorWithMetadata(
//	    "507f1f77bcf86cd799439011",
//	    bson.D{{"created_at", -1}},
//	    "_id",
//	    bson.M{"status": "active"},
//	    sortValue,
//	)
//	if err != nil {
//	    // Handle invalid parameters
//	}
func NewCursorWithMetadata(cursorId string, sort bson.D, cursorIdField string, filter bson.M, sortValue any) (*Cursor, error) {
	// Validate cursor ID format
	if !isValidCursorId(cursorId) {
		return nil, fmt.Errorf("invalid cursor id: must be a MongoDB ObjectID (24 hex characters)")
	}

	// Validate sort
	if len(sort) == 0 {
		return nil, fmt.Errorf("invalid sort: must not be empty")
	}

	// Validate cursor ID field
	if cursorIdField == "" {
		return nil, fmt.Errorf("invalid cursor id field: must not be empty")
	}

	// Encode sort to base64-encoded BSON
	encodedSort, err := encodeSortToString(sort)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "encode sort")
	}

	// Compute checksum for validation
	checksum, err := computeCursorChecksum(sort, cursorIdField)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "compute checksum")
	}

	// Optimize: don't store cursorIdField if it's the default value
	// This reduces cursor size for the most common case
	storedCursorIdField := cursorIdField
	if cursorIdField == DefaultCursorIdField {
		storedCursorIdField = "" // Will be omitted in JSON thanks to omitempty tag
	}

	// Optimize: don't store sort if it's the default sort (created_at descending)
	// This reduces cursor size for the most common case
	storedSort := encodedSort
	if isSortEqual(sort, DefaultListSort) {
		storedSort = "" // Will be omitted in JSON thanks to omitempty tag
	}

	// Compute filter hash for validation (always, even for empty filter)
	filterHash := computeFilterHash(filter)

	// Store SortValue only when sorting by a field different from cursorIdField.
	// This enables compound cursor filter for correct pagination.
	// Encode as base64 BSON to preserve MongoDB type precision.
	var storedSortValue string
	if len(sort) > 0 && sort[0].Key != cursorIdField {
		encoded, err := encodeSortValue(sortValue)
		if err != nil {
			return nil, coreerrs.WrapOperation(err, "encode sort value")
		}
		storedSortValue = encoded
	}

	return &Cursor{
		CursorId:      cursorId,
		Sort:          storedSort,
		CursorIdField: storedCursorIdField,
		Checksum:      checksum,
		FilterHash:    filterHash,
		SortValue:     storedSortValue,
	}, nil
}

// ValidateChecksum validates the cursor's checksum against the provided parameters.
// Returns an error if the checksum doesn't match, indicating that pagination parameters
// have changed between requests, or if the cursor doesn't contain required metadata.
//
// Parameters:
//   - sort: Expected MongoDB sort specification as bson.D
//   - cursorIdField: Expected MongoDB field name for cursor ID
//
// Returns:
//   - nil if checksum is valid
//   - error if cursor is nil, missing metadata, checksum doesn't match, or decoding fails
//
// Example:
//
//	if err := cursor.ValidateChecksum(bson.D{{"created_at", -1}}, "cursor_id"); err != nil {
//	    // Pagination parameters have changed or cursor is invalid
//	}
func (c *Cursor) ValidateChecksum(sort bson.D, cursorIdField string) error {
	if c == nil {
		return fmt.Errorf("cannot validate nil cursor")
	}

	// Cursor must have checksum
	if c.Checksum == "" {
		return fmt.Errorf("cursor is missing required metadata (checksum)")
	}

	// Decode cursor's sort for comparison (use default if empty)
	var cursorSort bson.D
	var err error
	if c.Sort != "" {
		cursorSort, err = decodeSortFromString(c.Sort)
		if err != nil {
			return coreerrs.WrapOperation(err, "decode cursor sort")
		}
	} else {
		// Empty sort means default sort was used
		cursorSort = DefaultListSort
	}

	// Compute expected checksum
	expectedChecksum, err := computeCursorChecksum(sort, cursorIdField)
	if err != nil {
		return coreerrs.WrapOperation(err, "compute checksum")
	}

	// Validate checksum
	if c.Checksum != expectedChecksum {
		// Get actual cursor ID field (use default if empty)
		actualCursorIdField := cmp.Or(c.CursorIdField, DefaultCursorIdField)

		return coreerrs.Wrapf(ErrCursorChecksumMismatch, "pagination parameters have changed (sort: %v→%v, cursor field: %s→%s)",
			cursorSort, sort,
			actualCursorIdField, cursorIdField)
	}

	return nil
}

// ValidateFilter validates the cursor's filter hash against the provided filter.
// Returns an error if the filter hash doesn't match, indicating that the query filter
// has changed between pagination requests.
//
// Parameters:
//   - filter: Current MongoDB filter to validate
//
// Returns:
//   - nil if filter hash matches
//   - error if cursor is nil, missing filter hash, or filter hash doesn't match
//
// Example:
//
//	if err := cursor.ValidateFilter(bson.M{"status": "active"}); err != nil {
//	    // Filter has changed mid-pagination
//	}
func (c *Cursor) ValidateFilter(filter bson.M) error {
	if c == nil {
		return fmt.Errorf("cannot validate nil cursor")
	}

	// FilterHash is required
	if c.FilterHash == "" {
		return fmt.Errorf("cursor is missing required filter hash")
	}

	// Compute current filter hash
	currentHash := computeFilterHash(filter)

	// Compare hashes
	if c.FilterHash != currentHash {
		return coreerrs.Wrapf(ErrCursorFilterMismatch, "expected hash %s, got %s",
			c.FilterHash, currentHash)
	}

	return nil
}

// GetSort decodes and returns the sort specification from the cursor.
// If the cursor has an empty sort field, returns DefaultListSort (optimization).
//
// Returns:
//   - bson.D: The sort specification (or DefaultListSort if empty)
//   - error: If decoding fails
//
// Example:
//
//	sort, err := cursor.GetSort()
//	if err != nil {
//	    // Handle error
//	}
func (c *Cursor) GetSort() (bson.D, error) {
	if c == nil {
		return nil, fmt.Errorf("cannot get sort from nil cursor")
	}

	// Empty sort means the default sort was used (optimization)
	if c.Sort == "" {
		return DefaultListSort, nil
	}

	return decodeSortFromString(c.Sort)
}
