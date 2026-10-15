// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// shortCtx bounds ListCursor calls against the detached collection: a request
// that wrongly gets past cursor validation fails on server selection instead
// of hanging for the driver's default timeout.
func shortCtx(t testing.TB) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	t.Cleanup(cancel)
	return ctx
}

// forgedCursor builds a stateless cursor whose checksum is computed for the
// server's sort and ID field (the checksum carries no secret) while the
// embedded metadata is attacker-chosen.
func forgedCursor(t testing.TB, serverSort bson.D, serverIDField string, embeddedSort bson.D, embeddedIDField string, filter bson.M) *Cursor {
	t.Helper()
	checksum, err := computeCursorChecksum(serverSort, serverIDField)
	require.NoError(t, err)
	encodedSort, err := encodeSortToString(embeddedSort)
	require.NoError(t, err)
	filterHash, err := computeFilterHash(filter)
	require.NoError(t, err)
	return &Cursor{
		CursorId:      bson.NewObjectID().Hex(),
		Sort:          encodedSort,
		CursorIdField: embeddedIDField,
		Checksum:      checksum,
		FilterHash:    filterHash,
	}
}

// TestParseCursorToken_StorageRejectsStatelessToken pins SB-003: with cursor
// storage configured, the token format comes from configuration, so a
// client-minted stateless token is rejected instead of bypassing storage.
func TestParseCursorToken_StorageRejectsStatelessToken(t *testing.T) {
	t.Parallel()

	filter := bson.M{"status": "active"}
	cursor, err := NewCursorWithMetadata(bson.NewObjectID().Hex(), DefaultListSort, DefaultCursorIdField, filter, int64(1))
	require.NoError(t, err)

	_, err = parseCursorToken(t.Context(), cursor.String(), newFakeCursorStorage(), filter, "tenant-A")
	require.ErrorIs(t, err, ErrInvalidCursorFormat)
}

// TestListCursor_StorageEndpointRejectsNonStorageCursors pins SB-003 end to
// end: neither an unsigned stateless token nor a direct *Cursor is accepted
// where cursor storage (and subject binding) is configured.
func TestListCursor_StorageEndpointRejectsNonStorageCursors(t *testing.T) {
	t.Parallel()

	coll := newDetachedCollection(t)
	filter := bson.M{"tenant": "A"}
	sort := bson.D{{Key: "name", Value: 1}}
	// Attacker-built token: checksum matches the server sort, metadata is forged.
	forged := forgedCursor(t, sort, DefaultCursorIdField, bson.D{{Key: "secret", Value: 1}}, "_id", filter)

	tests := []struct {
		name   string
		cursor ListCursorOption
	}{
		{name: "stateless token", cursor: WithListCursorCursor(forged.String())},
		{name: "direct cursor", cursor: WithListCursorCursor(forged)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := ListCursor[bson.M](shortCtx(t), coll,
				WithListCursorStorage(newFakeCursorStorage()),
				WithListCursorSubject("tenant-A"),
				WithListCursorFilter(filter),
				WithListCursorSort(sort),
				tc.cursor,
			)
			require.ErrorIs(t, err, ErrInvalidCursorFormat)
		})
	}
}

// TestCursor_ValidateChecksum_RejectsEditedMetadata pins SB-004: a checksum
// valid for the server parameters does not authorize different embedded
// sort or ID-field metadata.
func TestCursor_ValidateChecksum_RejectsEditedMetadata(t *testing.T) {
	t.Parallel()

	serverSort := bson.D{{Key: "name", Value: 1}}
	tests := []struct {
		name    string
		sort    bson.D
		idField string
		valid   bool
	}{
		{name: "matching metadata", sort: serverSort, idField: "", valid: true},
		{name: "edited sort field", sort: bson.D{{Key: "secret", Value: 1}}, idField: ""},
		{name: "edited sort direction", sort: bson.D{{Key: "name", Value: -1}}, idField: ""},
		{name: "edited id field", sort: serverSort, idField: "_id"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := forgedCursor(t, serverSort, DefaultCursorIdField, tc.sort, tc.idField, nil)
			err := c.ValidateChecksum(serverSort, DefaultCursorIdField)
			if tc.valid {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, ErrCursorChecksumMismatch)
		})
	}
}

// TestListCursor_RejectsEditedSortMetadata pins SB-004 at the ListCursor
// level: the forged sort is never adopted.
func TestListCursor_RejectsEditedSortMetadata(t *testing.T) {
	t.Parallel()

	coll := newDetachedCollection(t)
	sort := bson.D{{Key: "name", Value: 1}}
	forged := forgedCursor(t, sort, DefaultCursorIdField, bson.D{{Key: "secret", Value: 1}}, "", nil)

	_, err := ListCursor[bson.M](shortCtx(t), coll,
		WithListCursorSort(sort),
		WithListCursorCursor(forged.String()),
	)
	require.ErrorIs(t, err, ErrCursorChecksumMismatch)
}

// rawSortValue encodes v the way an attacker would, bypassing the type check
// in encodeSortValue.
func rawSortValue(t testing.TB, doc bson.D) string {
	t.Helper()
	b, err := bson.Marshal(doc)
	require.NoError(t, err)
	return base64.StdEncoding.EncodeToString(b)
}

// TestDecodeSortValue_RejectsNonScalar pins SB-004: a decoded SortValue must
// be a scalar, never a document, array, regex or code value that could act as
// a query operator, and the wrapper must be exactly {v: <value>}.
func TestDecodeSortValue_RejectsNonScalar(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		encoded string
	}{
		{name: "document with $regex", encoded: rawSortValue(t, bson.D{{Key: "v", Value: bson.D{{Key: "$regex", Value: ".*"}}}})},
		{name: "document with $ne", encoded: rawSortValue(t, bson.D{{Key: "v", Value: bson.M{"$ne": nil}}})},
		{name: "array", encoded: rawSortValue(t, bson.D{{Key: "v", Value: bson.A{1, 2}}})},
		{name: "regex", encoded: rawSortValue(t, bson.D{{Key: "v", Value: bson.Regex{Pattern: ".*"}}})},
		{name: "javascript", encoded: rawSortValue(t, bson.D{{Key: "v", Value: bson.JavaScript("return true")}})},
		{name: "code with scope", encoded: rawSortValue(t, bson.D{{Key: "v", Value: bson.CodeWithScope{Code: "x", Scope: bson.D{}}}})},
		{name: "symbol", encoded: rawSortValue(t, bson.D{{Key: "v", Value: bson.Symbol("x")}})},
		{name: "undefined", encoded: rawSortValue(t, bson.D{{Key: "v", Value: bson.Undefined{}}})},
		{name: "min key", encoded: rawSortValue(t, bson.D{{Key: "v", Value: bson.MinKey{}}})},
		{name: "duplicate v smuggles a document", encoded: rawSortValue(t, bson.D{
			{Key: "v", Value: "x"}, {Key: "v", Value: bson.D{{Key: "$regex", Value: ".*"}}},
		})},
		{name: "extra field", encoded: rawSortValue(t, bson.D{{Key: "v", Value: "x"}, {Key: "w", Value: "y"}})},
		{name: "missing v", encoded: rawSortValue(t, bson.D{{Key: "w", Value: "x"}})},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := decodeSortValue(tc.encoded)
			require.ErrorIs(t, err, ErrInvalidCursor)
		})
	}

	t.Run("malformed input", func(t *testing.T) {
		t.Parallel()
		for _, encoded := range []string{"%%%", base64.StdEncoding.EncodeToString([]byte{1, 2, 3})} {
			_, err := decodeSortValue(encoded)
			require.Error(t, err)
		}
	})

	t.Run("scalars round-trip", func(t *testing.T) {
		t.Parallel()
		values := []any{
			"x", int32(1), int64(2), 1.5, true, bson.NewObjectID(), bson.DateTime(1), bson.NewDecimal128(1, 2),
			bson.Timestamp{T: 1, I: 2}, bson.Binary{Subtype: 4, Data: []byte("0123456789abcdef")},
		}
		for _, v := range values {
			encoded, err := encodeSortValue(v)
			require.NoError(t, err)
			got, err := decodeSortValue(encoded)
			require.NoError(t, err)
			require.Equal(t, v, got)
		}
	})
}

// TestEncodeSortValue_RejectsNonScalar pins the generation side of SB-004: the
// server never issues a cursor whose SortValue the next request would reject.
func TestEncodeSortValue_RejectsNonScalar(t *testing.T) {
	t.Parallel()

	for _, v := range []any{bson.D{{Key: "a", Value: 1}}, bson.A{1}, bson.Regex{Pattern: "x"}} {
		_, err := encodeSortValue(v)
		require.ErrorIs(t, err, ErrInvalidCursor)
	}

	_, err := NewCursorWithMetadata(bson.NewObjectID().Hex(), bson.D{{Key: "name", Value: 1}}, DefaultCursorIdField, nil,
		bson.D{{Key: "$regex", Value: ".*"}})
	require.ErrorIs(t, err, ErrInvalidCursor)
}

// TestListCursor_RejectsInvalidSortValue pins SB-004 end to end: a cursor
// whose checksum and metadata are valid but whose SortValue is a document
// carrying $regex, or which carries a SortValue for an ID-led sort, is
// rejected before any query runs.
func TestListCursor_RejectsInvalidSortValue(t *testing.T) {
	t.Parallel()

	coll := newDetachedCollection(t)
	tests := []struct {
		name string
		sort bson.D
	}{
		{name: "document with $regex", sort: bson.D{{Key: "name", Value: 1}}},
		{name: "ID-led sort", sort: bson.D{{Key: DefaultCursorIdField, Value: 1}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cursor := forgedCursor(t, tc.sort, DefaultCursorIdField, tc.sort, "", nil)
			cursor.SortValue = rawSortValue(t, bson.D{{Key: "v", Value: bson.D{{Key: "$regex", Value: ".*"}}}})

			_, err := ListCursor[bson.M](shortCtx(t), coll,
				WithListCursorSort(tc.sort),
				WithListCursorCursor(cursor.String()),
			)
			require.ErrorIs(t, err, ErrInvalidCursor)
		})
	}
}

// TestListCursor_DirectCursorValidated pins that a direct *Cursor in stateless
// mode gets the same validation as a token, including the filter hash and the
// cursor ID format.
func TestListCursor_DirectCursorValidated(t *testing.T) {
	t.Parallel()

	coll := newDetachedCollection(t)
	sort := bson.D{{Key: "name", Value: 1}}

	t.Run("filter changed", func(t *testing.T) {
		t.Parallel()
		cursor := forgedCursor(t, sort, DefaultCursorIdField, sort, "", bson.M{"tenant": "A"})
		_, err := ListCursor[bson.M](shortCtx(t), coll,
			WithListCursorSort(sort),
			WithListCursorFilter(bson.M{"tenant": "B"}),
			WithListCursorCursor(cursor),
		)
		require.ErrorIs(t, err, ErrCursorFilterMismatch)
	})

	t.Run("invalid cursor id", func(t *testing.T) {
		t.Parallel()
		cursor := forgedCursor(t, sort, DefaultCursorIdField, sort, "", nil)
		cursor.CursorId = "not-an-object-id"
		_, err := ListCursor[bson.M](shortCtx(t), coll,
			WithListCursorSort(sort),
			WithListCursorCursor(cursor),
		)
		require.ErrorIs(t, err, ErrInvalidCursor)
	})
}

// TestStatefulCursor_SortValueRoundTrip pins that a storage-issued token keeps
// the sort value's BSON type through Store → Load → ToCursor → filter, instead
// of re-encoding the stored base64 as a string.
func TestStatefulCursor_SortValueRoundTrip(t *testing.T) {
	t.Parallel()

	storage := newFakeCursorStorage()
	sort := bson.D{{Key: "created_at", Value: -1}}
	filter := bson.M{"status": "active"}
	oid := bson.NewObjectID()
	sortValue := bson.DateTime(1_700_000_000_000)

	token, err := generateNextCursorToken(t.Context(), oid.Hex(), sort, DefaultCursorIdField, filter, storage, sortValue, "tenant-A")
	require.NoError(t, err)

	cursor, err := parseCursorToken(t.Context(), token, storage, filter, "tenant-A")
	require.NoError(t, err)
	require.NoError(t, validateCursorSortValue(cursor, sort, DefaultCursorIdField))

	got := buildCursorFilter(cursor, DefaultCursorIdField, sort)
	want := bson.M{"$or": bson.A{
		bson.M{"created_at": bson.M{"$lt": sortValue}},
		bson.M{"$and": bson.A{
			bson.M{"created_at": bson.M{"$eq": sortValue}},
			bson.M{DefaultCursorIdField: bson.M{"$lt": oid}},
		}},
	}}
	require.Equal(t, want, got)
}

// TestBuildCursorFilter_EqualityUsesEq pins SB-004: the tie-breaker equality
// predicate is rendered with an explicit $eq so the value is never parsed as
// a query expression.
func TestBuildCursorFilter_EqualityUsesEq(t *testing.T) {
	t.Parallel()

	sortVal, err := encodeSortValue("alice")
	require.NoError(t, err)
	oid := bson.NewObjectID()
	got := buildCursorFilter(&Cursor{CursorId: oid.Hex(), SortValue: sortVal}, DefaultCursorIdField,
		bson.D{{Key: "name", Value: 1}})

	want := bson.M{"$or": bson.A{
		bson.M{"name": bson.M{"$gt": "alice"}},
		bson.M{"$and": bson.A{
			bson.M{"name": bson.M{"$eq": "alice"}},
			bson.M{DefaultCursorIdField: bson.M{"$gt": oid}},
		}},
	}}
	require.Equal(t, want, got)
}
