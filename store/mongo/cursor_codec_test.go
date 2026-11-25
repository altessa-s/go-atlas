// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-aip/pagination/pagetoken"
)

// TestCursorCodecLegacyWire checks issuance against data from the historical
// writer. TestLegacyListCursorFixtures exercises the reader through ListCursor.
func TestCursorCodecLegacyWire(t *testing.T) {
	t.Parallel()
	var fixture struct {
		Signed string
		Record CursorMetadata
	}
	data, err := os.ReadFile("testdata/listcursor/legacy.json")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &fixture))
	signer := newCursorSigner(t, 1, pagetoken.WithClock(func() time.Time { return time.Unix(1700000000, 0) }))
	// An unprepared bson.M uses the legacy JSON filter digest.
	opts := newListCursorOptions(signedRequest(signer, fixture.Record.Subject)...)
	issue := func(o *listCursorOptions) string {
		token, err := o.nextCursorToken(t.Context(), fixture.Record.CursorId, "alice", fixture.Record.Collection)
		require.NoError(t, err)
		return token
	}
	require.Equal(t, fixture.Signed, issue(opts))

	const unsigned = "eyJjIjoiNjU1M2YxMDAwMDAwMDAwMDAwMDAwMDAxIiwicyI6IkR3QUFBQkJ1WVcxbEFBRUFBQUFBIiwiY3MiOiJiMzMxZTA0" +
		"ZmRjZDQ4NWViMzZiMzg2OGJlYzljNGE5ZGJjZTliZmMyODNmOTI0N2M1MGFkYzY4Mjk5YjAwYzgwIiwiZmgiOiJmZmNjOTg3" +
		"MGE3NTFhMDI0MWY1ZjJiZGFjOGU2NjQ2YzQwYjkyYmIyMjZlOGVmYzRhZjJlMjljYzI0MmZjMTc2Iiwic3YiOiJFZ0FBQUFK" +
		"MkFBWUFBQUJoYkdsalpRQUEifQ=="
	plain := newListCursorOptions(WithListCursorSort("name"), WithListCursorFilter(bson.M{"status": "active"}))
	require.Equal(t, unsigned, issue(plain))
	c, err := ParseCursor(unsigned)
	require.NoError(t, err)
	require.Equal(t, unsigned, c.String())

	storage := newFakeCursorStorage()
	stored := newListCursorOptions(append(signedRequest(signer, fixture.Record.Subject), WithListCursorStorage(storage))...)
	key := issue(stored)
	require.True(t, isULID(key))
	metadata, err := storage.Load(t.Context(), key)
	require.NoError(t, err)
	require.False(t, metadata.CreatedAt.IsZero())
	fixture.Record.CreatedAt = metadata.CreatedAt
	require.Equal(t, fixture.Record, *metadata)
}

// TestPreparedCursorWire exercises issuance and continuation through ListCursor.
// prepared.json was captured from the pre-refactor writer with a fixed signer clock.
func TestPreparedCursorWire(t *testing.T) {
	t.Parallel()
	var wire struct {
		Unsigned, Signed string
		Record           json.RawMessage
	}
	data, err := os.ReadFile("testdata/listcursor/prepared.json")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &wire))
	const id = "6553f1000000000000000001"
	batch := bson.A{bson.M{"items": bson.A{
		bson.M{"cursor_id": id, "name": "alice"},
		bson.M{"cursor_id": "6553f1000000000000000002", "name": "bob"},
	}}}
	signer := newCursorSigner(t, 1, pagetoken.WithClock(func() time.Time { return time.Unix(1700000000, 0) }))
	for _, mode := range []string{"unsigned", "signed", "stored"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			opts := []ListCursorOption{WithListCursorFilter(bson.M{"status": "active"}), WithListCursorSort("name"), WithListCursorLimit(1)}
			storage := newFakeCursorStorage()
			if mode != "unsigned" {
				opts = append(opts, WithListCursorSigner(signer), WithListCursorSubject("tenant-A"))
			}
			if mode == "stored" {
				opts = append(opts, WithListCursorStorage(storage))
			}
			first, err := ListCursor[bson.M](t.Context(), mockReadCollection(t, nil, batch), opts...)
			require.NoError(t, err)
			require.NotNil(t, first.NextCursor)
			token := *first.NextCursor
			switch mode {
			case "unsigned":
				require.Equal(t, wire.Unsigned, token)
			case "signed":
				require.Equal(t, wire.Signed, token)
			case "stored":
				require.True(t, isULID(token))
				got, err := storage.Load(t.Context(), token)
				require.NoError(t, err)
				require.False(t, got.CreatedAt.IsZero())
				var expected CursorMetadata
				require.NoError(t, json.Unmarshal(wire.Record, &expected))
				got.CreatedAt = expected.CreatedAt
				encoded, err := json.Marshal(got)
				require.NoError(t, err)
				var compact bytes.Buffer
				require.NoError(t, json.Compact(&compact, wire.Record))
				require.Equal(t, compact.String(), string(encoded))
				require.NoError(t, storage.Store(t.Context(), token, &expected))
			}
			next, err := ListCursor[bson.M](t.Context(), mockReadCollection(t, nil, bson.A{}), append(opts, WithListCursorCursor(token))...)
			require.NoError(t, err)
			require.Nil(t, next.NextCursor)
		})
	}
}

func TestListCursorSeqSignedContinuation(t *testing.T) {
	t.Parallel()
	type item struct {
		ID bson.ObjectID `bson:"cursor_id"`
	}
	items := []item{{bson.NewObjectID()}, {bson.NewObjectID()}, {bson.NewObjectID()}}
	page := func(items []item) bson.A { return bson.A{bson.M{"items": items}} }
	coll := mockReadCollection(t, nil, page(items[:2]), page(items[1:]), page(items[2:]))
	var got []item
	for item, err := range ListCursorSeq[item](t.Context(), coll, WithListCursorSigner(newCursorSigner(t, 1)),
		WithListCursorSort("cursor_id"), WithListCursorLimit(1)) {
		require.NoError(t, err)
		got = append(got, item)
	}
	require.Equal(t, items, got)
}
