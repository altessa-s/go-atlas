// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	corekeys "github.com/altessa-s/go-atlas-core/encoding/keys"
	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

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

// cursorCollectionDigest identifies the collection a stateful cursor is bound
// to: a length-prefixed digest of the database and collection names, so the
// pairs ("a", "b.c") and ("a.b", "c") differ.
func cursorCollectionDigest(collection *mongo.Collection) string {
	return hex.EncodeToString(corekeys.Digest([]byte(collection.Database().Name()), []byte(collection.Name())))
}

// validateStatelessCursor checks a client-supplied stateless cursor against
// the server's pagination parameters: its ID must be an ObjectID, its checksum
// and embedded sort/ID-field metadata must match sort and cursorIdField, and —
// for a direct *Cursor, supplied without a token — its filter hash must
// match filter.
func validateStatelessCursor(ctx context.Context, c *Cursor, sort bson.D, cursorIdField string, filter cursorFilter, direct bool) error {
	if !isValidCursorId(c.CursorId) {
		return coreerrs.Wrap(ErrInvalidCursor, "cursor_id must be a MongoDB ObjectID (24 hex characters)")
	}
	if err := c.ValidateChecksum(sort, cursorIdField); err != nil {
		return err
	}
	if direct {
		return c.validateFilter(ctx, filter)
	}
	return nil
}

// validateCursorSortValue checks the cursor's SortValue against the effective
// sort: the server stores one only when the sort is led by a field other than
// cursorIdField, and it must decode to a supported BSON scalar.
func validateCursorSortValue(c *Cursor, sort bson.D, cursorIdField string) error {
	if len(sort) == 0 || sort[0].Key == cursorIdField {
		if c.SortValue != "" {
			return coreerrs.Wrap(ErrInvalidCursor, "sort value present for a sort led by the cursor ID field")
		}
		return nil
	}
	if c.SortValue == "" {
		return coreerrs.Wrap(ErrInvalidCursor, "sort value missing for a sort led by a non-ID field")
	}
	if _, err := decodeSortValue(c.SortValue); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidCursor, err)
	}
	return nil
}

// extractCursorBoundary reads the wire values before the result model's codec
// can transform or omit them.
func extractCursorBoundary(doc bson.Raw, cursorIDField string, sort bson.D) (string, any, error) {
	id, ok := lookupCursorPath(doc, cursorIDField).ObjectIDOK()
	if !ok {
		return "", nil, coreerrs.Wrap(ErrMissingCursorKey, "cursor ID must be a BSON ObjectID")
	}
	if len(sort) == 0 || sort[0].Key == cursorIDField {
		return id.Hex(), nil, nil
	}

	value := lookupCursorPath(doc, sort[0].Key)
	if err := checkSortValueType(value.Type); err != nil {
		return "", nil, fmt.Errorf("%w: %w", ErrMissingCursorKey, err)
	}
	return id.Hex(), value, nil
}

func lookupCursorPath(doc bson.Raw, path string) bson.RawValue {
	for {
		field, rest, nested := strings.Cut(path, ".")
		value := doc.Lookup(field)
		switch {
		case value.IsZero():
			return bson.RawValue{Type: bson.TypeNull}
		case !nested, value.Type == bson.TypeArray:
			// A multikey sort boundary cannot be represented by a scalar.
			return value
		case value.Type == bson.TypeEmbeddedDocument:
			doc, path = value.Document(), rest
		default:
			return bson.RawValue{Type: bson.TypeNull}
		}
	}
}
