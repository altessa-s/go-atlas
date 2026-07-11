// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"encoding/json"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	corehash "github.com/altessa-s/go-atlas/core/encoding/hash"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// CursorMetadata contains all cursor information stored server-side.
// This struct is stored in CursorStorage implementations (Redis, MongoDB, Memory, etc.)
// and contains everything needed to continue pagination from where it left off.
//
// Server-side storage provides:
//   - Security: Clients cannot tamper with sort order or field names
//   - Validation: Filter hash ensures query consistency between requests
//   - Smaller tokens: Client receives only a storage key instead of full metadata
type CursorMetadata struct {
	// CursorId is the MongoDB ObjectID from the last item in the previous page.
	// This is used to build the $gt/$lt filter for the next page.
	// Must be a valid ObjectID (24 hexadecimal characters).
	CursorId string `json:"cursor_id"`

	// Sort is the base64-encoded BSON representation of the sort specification.
	// Stored as string to avoid BSON marshaling issues in storage implementations.
	// Example: base64(bson.Marshal(bson.D{{"created_at", -1}}))
	Sort string `json:"sort"`

	// CursorIdField is the MongoDB field name containing the cursor ID.
	// Usually "cursor_id" or "_id".
	// This must remain consistent across pagination requests.
	CursorIdField string `json:"cursor_id_field"`

	// FilterHash is a SHA-256 hash of the original filter (bson.M).
	// Used to validate that the filter hasn't changed between pagination requests.
	// Prevents clients from changing query filters mid-pagination.
	// Format: 64-character hex-encoded SHA-256 hash.
	FilterHash string `json:"filter_hash"`

	// CreatedAt is the timestamp when this cursor was created.
	// Storage implementations can use this for TTL calculations and cleanup.
	// Also useful for debugging and monitoring cursor usage patterns.
	CreatedAt time.Time `json:"created_at"`

	// SortValue stores the base64-encoded BSON value of the sort field.
	// Stored as string to preserve MongoDB type precision (Decimal128, Date, ObjectID).
	// Stored only if sorting by a different field than CursorIdField.
	// Used to build the $gt/$lt filter for the next page.
	// Omitted if sorting by CursorIdField to save space.
	SortValue string `json:"sort_value,omitempty"`

	// Subject binds the cursor to the principal that created it (e.g. tenant or
	// user id). When set, ValidateSubject rejects a continuation request whose
	// subject differs, so a stateful cursor token leaked or guessed by another
	// principal cannot be replayed to page through the original principal's data.
	// Empty means unbound — the backward-compatible default when no subject is
	// configured (see WithListCursorSubject).
	Subject string `json:"subject,omitempty"`
}

// NewCursorMetadata creates cursor metadata from pagination parameters.
// This is called internally when storing a cursor in server-side storage.
//
// Parameters:
//   - cursorId: MongoDB ObjectID from the last item (24 hex characters)
//   - sort: MongoDB sort specification
//   - cursorIdField: Field name for cursor ID (e.g., "cursor_id", "_id")
//   - filter: Original query filter for validation
//   - sortValue: Value of the sort field from the last item (nil if sorting by cursorIdField)
//
// Returns:
//   - *CursorMetadata: Ready to store in CursorStorage
//   - error: If encoding fails
func NewCursorMetadata(cursorId string, sort bson.D, cursorIdField string, filter bson.M, sortValue any) (*CursorMetadata, error) {
	// Encode sort to base64 BSON
	encodedSort, err := encodeSortToString(sort)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "encode sort")
	}

	// Compute filter hash for validation
	filterHash, err := computeFilterHash(filter)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "compute filter hash")
	}

	// Store sort field value only if sorting by different field
	// Encode as base64 BSON to preserve MongoDB type precision
	var storedSortFieldValue string
	if len(sort) > 0 && sort[0].Key != cursorIdField {
		encoded, err := encodeSortValue(sortValue)
		if err != nil {
			return nil, coreerrs.WrapOperation(err, "encode sort value")
		}
		storedSortFieldValue = encoded
	}

	return &CursorMetadata{
		CursorId:      cursorId,
		Sort:          encodedSort,
		CursorIdField: cursorIdField,
		FilterHash:    filterHash,
		CreatedAt:     time.Now(),
		SortValue:     storedSortFieldValue,
	}, nil
}

// GetSort decodes the sort specification from base64 BSON.
//
// Returns:
//   - bson.D: Decoded sort specification
//   - error: If decoding fails
func (m *CursorMetadata) GetSort() (bson.D, error) {
	if m.Sort == "" {
		return nil, fmt.Errorf("sort is empty")
	}
	return decodeSortFromString(m.Sort)
}

// ValidateFilter checks if the provided filter matches the stored filter hash.
// This ensures the client hasn't changed the query filter between pagination requests.
//
// Parameters:
//   - filter: Current filter to validate
//
// Returns:
//   - error: ErrCursorFilterMismatch if filter has changed, nil if valid
func (m *CursorMetadata) ValidateFilter(filter bson.M) error {
	currentHash, err := computeFilterHash(filter)
	if err != nil {
		return coreerrs.WrapOperation(err, "compute filter hash")
	}
	if m.FilterHash != currentHash {
		return coreerrs.Wrapf(ErrCursorFilterMismatch, "expected hash %s, got %s",
			m.FilterHash, currentHash)
	}
	return nil
}

// ValidateSubject checks that the continuation request comes from the same
// principal that created the cursor. It is opt-in: when the stored Subject is
// empty (no subject was bound at creation), any caller is allowed, preserving
// backward compatibility. When the stored Subject is non-empty, a differing
// subject returns [ErrCursorSubjectMismatch] so a leaked or guessed cursor
// token cannot be replayed by another principal.
//
// Parameters:
//   - subject: Identity of the requesting principal (typically tenant/user id)
//
// Returns:
//   - error: ErrCursorSubjectMismatch if the cursor is bound to a different subject
func (m *CursorMetadata) ValidateSubject(subject string) error {
	if m.Subject == "" {
		return nil
	}
	if m.Subject != subject {
		return coreerrs.Wrap(ErrCursorSubjectMismatch, "stateful cursor bound to a different principal")
	}
	return nil
}

// ToCursor converts metadata back to a Cursor instance.
// This is used when loading cursor from storage to continue pagination.
//
// Returns:
//   - *Cursor: Cursor instance with metadata
//   - error: If conversion fails
func (m *CursorMetadata) ToCursor() (*Cursor, error) {
	// Validate cursor ID format
	if !isValidCursorId(m.CursorId) {
		return nil, fmt.Errorf("invalid cursor id in metadata: must be ObjectID (24 hex characters)")
	}

	// Decode sort for validation
	sort, err := m.GetSort()
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "decode sort from metadata")
	}

	// Validate cursor ID field
	if m.CursorIdField == "" {
		return nil, fmt.Errorf("cursor id field is empty in metadata")
	}

	// Create cursor with metadata using existing function
	// Note: We don't have the actual filter here, only its hash
	// This is fine because ToCursor is only used in stateful mode (server-side storage)
	// where filter validation is done via CursorMetadata.ValidateFilter()
	cursor, err := NewCursorWithMetadata(m.CursorId, sort, m.CursorIdField, nil, m.SortValue)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "create cursor from metadata")
	}

	return cursor, nil
}

// emptyFilterHash is the precomputed hash returned by computeFilterHash for
// nil or empty filters. The empty-filter branch is hot — list endpoints without
// a filter take it on every cursor write and ValidateFilter call.
var emptyFilterHash = corehash.SHA256HexString("")

// computeFilterHash creates a deterministic SHA-256 hash from a MongoDB filter.
// This is used for validating that filters haven't changed between pagination requests.
//
// The function ensures deterministic hashing by marshaling the filter to JSON,
// which sorts map keys at every nesting level (Go map iteration order is random).
//
// Parameters:
//   - filter: MongoDB filter using bson.M syntax
//
// Returns:
//   - string: 64-character hex-encoded SHA-256 hash; empty filter returns a fixed hash.
//   - error: [ErrCursorFilterUnmarshalable] wrapped with the underlying json.Marshal error
//     when the filter contains a value json cannot encode (NaN/Inf, channel, function,
//     cycle, or a MarshalJSON that returns an error). Cursor construction/validation
//     surface this so two distinct bad filters never silently share a hash.
//
// Example:
//   - computeFilterHash(bson.M{"age": 25, "name": "John"})
//     → SHA-256 of `{"age":25,"name":"John"}`, nil
func computeFilterHash(filter bson.M) (string, error) {
	if len(filter) == 0 {
		return emptyFilterHash, nil
	}

	// Marshal to JSON for a deterministic key order across calls.
	b, err := json.Marshal(filter)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrCursorFilterUnmarshalable, err)
	}
	return corehash.SHA256HexString(string(b)), nil
}
