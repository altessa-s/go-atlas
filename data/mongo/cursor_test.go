// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const validObjectID = "507f1f77bcf86cd799439011"

func TestNewCursor(t *testing.T) {
	t.Parallel()

	t.Run("valid ObjectID", func(t *testing.T) {
		t.Parallel()
		cursor, err := NewCursor(validObjectID)
		require.NoError(t, err)
		require.Equal(t, validObjectID, cursor.CursorId)
		require.Empty(t, cursor.Sort)
		require.Empty(t, cursor.Checksum)
	})

	t.Run("uppercase hex is valid", func(t *testing.T) {
		t.Parallel()
		cursor, err := NewCursor("507F1F77BCF86CD799439011")
		require.NoError(t, err)
		require.Equal(t, "507F1F77BCF86CD799439011", cursor.CursorId)
	})

	invalid := []struct {
		name     string
		cursorId string
	}{
		{"empty", ""},
		{"too short", "507f1f77bcf86cd79943901"},
		{"too long", "507f1f77bcf86cd7994390111"},
		{"non-hex character", "507f1f77bcf86cd79943901z"},
		{"ulid not accepted", "01ARZ3NDEKTSV4RRFFQ69G5FAV"},
	}

	for _, tt := range invalid {
		t.Run("invalid "+tt.name, func(t *testing.T) {
			t.Parallel()
			cursor, err := NewCursor(tt.cursorId)
			require.Error(t, err)
			require.Nil(t, cursor)
		})
	}
}

func TestMustNewCursor(t *testing.T) {
	t.Parallel()

	t.Run("valid ObjectID returns cursor", func(t *testing.T) {
		t.Parallel()
		cursor := MustNewCursor(validObjectID)
		require.Equal(t, validObjectID, cursor.CursorId)
	})

	t.Run("invalid ObjectID panics", func(t *testing.T) {
		t.Parallel()
		require.Panics(t, func() {
			MustNewCursor("not-an-object-id")
		})
	})
}

func TestCursorStringParseCursorRoundTrip(t *testing.T) {
	t.Parallel()

	t.Run("minimal cursor", func(t *testing.T) {
		t.Parallel()
		original := MustNewCursor(validObjectID)

		encoded := original.String()
		require.NotEmpty(t, encoded)

		decoded, err := ParseCursor(encoded)
		require.NoError(t, err)
		require.Equal(t, original, decoded)
	})

	t.Run("cursor with full metadata", func(t *testing.T) {
		t.Parallel()
		original, err := NewCursorWithMetadata(
			validObjectID,
			bson.D{{Key: "name", Value: SortAscending}},
			"_id",
			bson.M{"status": "active"},
			"last-name-value",
		)
		require.NoError(t, err)
		require.NotEmpty(t, original.Sort, "non-default sort must be stored")
		require.NotEmpty(t, original.Checksum)
		require.NotEmpty(t, original.FilterHash)
		require.NotEmpty(t, original.SortValue)

		decoded, err := ParseCursor(original.String())
		require.NoError(t, err)
		require.Equal(t, original, decoded)
	})

	t.Run("nil cursor encodes to empty string", func(t *testing.T) {
		t.Parallel()
		var c *Cursor
		require.Empty(t, c.String())
	})
}

func TestParseCursor_Errors(t *testing.T) {
	t.Parallel()

	b64 := func(s string) string {
		return base64.URLEncoding.EncodeToString([]byte(s))
	}

	tests := []struct {
		name    string
		encoded string
	}{
		{"empty string", ""},
		{"invalid base64", "%%%not-base64%%%"},
		{"base64 of non-JSON", b64("not json at all")},
		{"missing cursor id", b64(`{"s":"whatever"}`)},
		{"invalid cursor id format", b64(`{"c":"zzz"}`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cursor, err := ParseCursor(tt.encoded)
			require.ErrorIs(t, err, ErrInvalidCursor)
			require.Nil(t, cursor)
		})
	}
}

func TestCursorJSONRoundTrip(t *testing.T) {
	t.Parallel()

	t.Run("marshal produces quoted token, unmarshal restores cursor", func(t *testing.T) {
		t.Parallel()
		original := MustNewCursor(validObjectID)

		data, err := json.Marshal(original)
		require.NoError(t, err)

		var quoted string
		require.NoError(t, json.Unmarshal(data, &quoted))
		require.Equal(t, original.String(), quoted)

		var decoded Cursor
		require.NoError(t, json.Unmarshal(data, &decoded))
		require.Equal(t, *original, decoded)
	})

	t.Run("marshal nil cursor yields null", func(t *testing.T) {
		t.Parallel()
		var c *Cursor
		data, err := c.MarshalJSON()
		require.NoError(t, err)
		require.JSONEq(t, "null", string(data))
	})

	t.Run("unmarshal invalid token fails", func(t *testing.T) {
		t.Parallel()
		var decoded Cursor
		err := json.Unmarshal([]byte(`"%%%not-a-cursor%%%"`), &decoded)
		require.ErrorIs(t, err, ErrInvalidCursor)
	})

	t.Run("unmarshal non-string JSON fails", func(t *testing.T) {
		t.Parallel()
		var decoded Cursor
		require.Error(t, json.Unmarshal([]byte(`123`), &decoded))
	})
}
