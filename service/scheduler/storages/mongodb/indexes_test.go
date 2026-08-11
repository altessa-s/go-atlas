// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// White-box: the index models and the document structs they must agree with
// are unexported, and the whole point of this file is to compare the two.
package mongodb

import (
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// bsonFields returns the set of bson field names a document struct persists.
func bsonFields(tb testing.TB, doc any) map[string]struct{} {
	tb.Helper()

	t := reflect.TypeOf(doc)
	require.Equal(tb, reflect.Struct, t.Kind(), "document must be a struct")

	fields := make(map[string]struct{}, t.NumField())
	for i := range t.NumField() {
		tag, ok := t.Field(i).Tag.Lookup("bson")
		require.Truef(tb, ok, "field %s has no bson tag", t.Field(i).Name)
		name, _, _ := strings.Cut(tag, ",")
		fields[name] = struct{}{}
	}
	return fields
}

// indexKeys flattens the key names of every index model.
func indexKeys(tb testing.TB, models []mongo.IndexModel) []string {
	tb.Helper()

	var keys []string
	for _, m := range models {
		doc, ok := m.Keys.(bson.D)
		require.True(tb, ok, "index keys must be a bson.D")
		for _, e := range doc {
			keys = append(keys, e.Key)
		}
	}
	return keys
}

// TestIndexKeys pins every index key to a field the corresponding document
// actually persists. MongoDB accepts an index on a field no document carries
// and then never uses it, so the mismatch is invisible at runtime — it only
// shows up as queries that silently fall back to a collection scan, or (as
// with the earlier start_time index) as a sort that returns rows in arbitrary
// order while the godoc promises otherwise.
func TestIndexKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		models []mongo.IndexModel
		doc    any
	}{
		{name: "tasks", models: taskIndexModels(), doc: taskDocument{}},
		{name: "history", models: historyIndexModels(), doc: historyDocument{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fields := bsonFields(t, tt.doc)
			for _, key := range indexKeys(t, tt.models) {
				require.Containsf(t, fields, key,
					"index key %q does not match any bson tag on %T", key, tt.doc)
			}
		})
	}
}

// TestHistoryIndexCoversSorts asserts the history index leads with the exact
// prefix both History and HistoryPaginated sort by, so neither degrades into
// an in-memory sort.
func TestHistoryIndexCoversSorts(t *testing.T) {
	t.Parallel()

	want := []string{"task_id", "started_at", "_id"}
	require.Equal(t, want, indexKeys(t, historyIndexModels())[:len(want)])
}

// TestNoTTLIndex guards the removal of the TTL index that never fired.
// EndedAt is a Unix timestamp stored as int64; MongoDB TTL indexes expire only
// documents whose indexed field holds a BSON date, so a TTL index here would
// silently retain history forever while looking like retention was handled.
// Cleanup is Storage.CleanupHistory's job.
func TestNoTTLIndex(t *testing.T) {
	t.Parallel()

	for _, m := range historyIndexModels() {
		if m.Options == nil {
			continue
		}
		var opts mongoOptions.IndexOptions
		for _, set := range m.Options.List() {
			require.NoError(t, set(&opts))
		}
		require.Nil(t, opts.ExpireAfterSeconds,
			"history retention must go through CleanupHistory, not a TTL index")
	}
}
