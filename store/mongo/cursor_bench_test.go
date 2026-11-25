// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func BenchmarkValidateStatelessCursor(b *testing.B) {
	sort := bson.D{{Key: "name", Value: 1}}
	filter := bson.M{"status": "active"}
	c, err := NewCursorWithMetadata(bson.NewObjectID().Hex(), sort, DefaultCursorIdField, filter, "alice")
	if err != nil {
		b.Fatal(err)
	}
	for b.Loop() {
		if err := validateStatelessCursor(b.Context(), c, sort, DefaultCursorIdField, cursorFilter{original: filter}, true); err != nil {
			b.Fatal(err)
		}
		if err := validateCursorSortValue(c, sort, DefaultCursorIdField); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCursorContinuation(b *testing.B) {
	storage := newFakeCursorStorage()
	for _, tc := range []struct {
		name    string
		options []ListCursorOption
	}{
		{"unsigned", nil},
		{"signed", []ListCursorOption{WithListCursorSigner(newCursorSigner(b, 1))}},
		{"stored", []ListCursorOption{WithListCursorStorage(storage)}},
	} {
		b.Run(tc.name, func(b *testing.B) {
			opts := newListCursorOptions(tc.options...)
			id := bson.NewObjectID().Hex()
			initial := *opts
			for b.Loop() {
				*opts = initial
				token, err := opts.nextCursorToken(b.Context(), id, nil, "collection")
				if err != nil {
					b.Fatal(err)
				}
				opts.cursorToken = token
				if err := resolveCursorOptions(b.Context(), opts, "collection"); err != nil {
					b.Fatal(err)
				}
				delete(storage.entries, token)
			}
		})
	}
}
