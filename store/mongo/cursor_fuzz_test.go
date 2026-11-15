// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"encoding/hex"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

// objectIDFrom renders arbitrary bytes as a well-formed 24-character ObjectID,
// so a round-trip target exercises the cursor body rather than spending every
// execution rejected by the ID check.
func objectIDFrom(seed []byte) string {
	buf := make([]byte, 12)
	copy(buf, seed)
	return hex.EncodeToString(buf)
}

// FuzzParseCursor feeds arbitrary strings to the pagination cursor decoder.
//
// The cursor is round-tripped through the client: it is emitted in a response,
// held by whoever is paging, and handed back on the next request, so its bytes
// are attacker-controlled by construction. It is also unauthenticated — base64
// over JSON, no signature — which makes what survives parsing the only thing
// standing between a hostile cursor and the query built from it.
//
// The assertion is the invariant the decoder documents: a cursor that parses
// carries a real MongoDB ObjectID. Everything downstream treats CursorId as one.
func FuzzParseCursor(f *testing.F) {
	f.Add("")
	f.Add("eyJjIjoiNTA3ZjFmNzdiY2Y4NmNkNzk5NDM5MDExIn0=") // Valid.
	f.Add("!!!not base64!!!")
	f.Add("eyJjIjoiIn0=")                             // Empty cursor_id.
	f.Add("eyJjIjoienp6enp6enp6enp6enp6enp6enp6eiJ9") // 24 non-hex characters.
	f.Add("eyJjIjpudWxsfQ==")                         // cursor_id: null.
	f.Add("eyJjIjp7fX0=")                             // cursor_id: object, not string.

	f.Fuzz(func(t *testing.T, encoded string) {
		cursor, err := ParseCursor(encoded)
		if err != nil {
			require.Nil(t, cursor, "a rejected cursor must not also be returned")
			return
		}

		require.NotNil(t, cursor)
		require.Len(t, cursor.CursorId, objectIDLength,
			"ParseCursor accepted an ID the rest of the package will treat as an ObjectID")
		_, hexErr := hex.DecodeString(cursor.CursorId)
		require.NoError(t, hexErr, "ParseCursor accepted a non-hexadecimal ObjectID")
	})
}

// FuzzCursorRoundTrip pins encode/decode agreement across the whole cursor
// body, not just its ID.
//
// The fields carry base64-encoded BSON (Sort, SortValue) and hashes that later
// requests are validated against. A cursor that does not survive its own
// String() unchanged means a client paging normally is told its parameters
// changed — pagination breaks for a well-behaved caller, which is the failure
// mode nobody reports as a security bug and everybody works around.
//
// The wire format is JSON, and encoding/json substitutes U+FFFD for any byte
// sequence that is not valid UTF-8. Exact round-tripping is therefore claimed
// only for UTF-8 fields — which is what every producer in this package emits,
// since all five are base64, hex, or a MongoDB field name. Non-UTF-8 input is
// still fed through, asserting the weaker property that it parses and keeps the
// pagination position; only the exact-equality half is skipped for it.
func FuzzCursorRoundTrip(f *testing.F) {
	f.Add([]byte{0x50, 0x7f, 0x1f, 0x77}, "", "", "", "")
	f.Add([]byte{0x01}, "c29ydA==", "cursor_id", "abc123", "dmFs")
	f.Add([]byte{}, "", "_id", "", "")
	f.Add([]byte{0xff, 0xff}, "\x00", "field.with.dots", "\n\t", "=")
	f.Add([]byte("0"), "", "", "", "\x86") // Lone continuation byte: not UTF-8.

	f.Fuzz(func(t *testing.T, idSeed []byte, sort, idField, checksum, sortValue string) {
		original := &Cursor{
			CursorId:      objectIDFrom(idSeed),
			Sort:          sort,
			CursorIdField: idField,
			Checksum:      checksum,
			SortValue:     sortValue,
		}

		decoded, err := ParseCursor(original.String())
		require.NoError(t, err, "a cursor this package produced must be one it accepts")
		require.Equal(t, original.CursorId, decoded.CursorId,
			"the pagination position changed on its way through the wire format")

		if !utf8.ValidString(sort) || !utf8.ValidString(idField) ||
			!utf8.ValidString(checksum) || !utf8.ValidString(sortValue) {
			return // JSON cannot carry these bytes unchanged; see the doc comment.
		}

		require.Equal(t, original, decoded, "the cursor changed on its way through the wire format")
	})
}
