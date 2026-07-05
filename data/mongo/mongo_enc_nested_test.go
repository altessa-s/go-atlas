// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// nestedChildWithOptional has both a required and an optional (nil-able) field
// so the recursion produces a nested $set (value) and a nested $unset (optional).
type nestedChildWithOptional struct {
	Value    string  `bson:"value"`
	Optional *string `bson:"optional"`
}

type nestedParentOptionalEntity struct {
	ID    string                   `bson:"_id"`
	Child *nestedChildWithOptional `bson:"child"`
}

type nestedMapParentEntity struct {
	ID   string                             `bson:"_id"`
	Kids map[string]nestedChildWithOptional `bson:"kids"`
}

// TestConvertToUpdateDocument_NestedStructPointer_WholeObjectReplacement pins the
// documented contract: a pointer-to-struct field is written as a single $set
// subdocument (whole-object replacement). A nested nil field is omitted from that
// subdocument — no per-field nested $unset is emitted.
func TestConvertToUpdateDocument_NestedStructPointer_WholeObjectReplacement(t *testing.T) {
	t.Parallel()

	m, err := New("testdb")
	require.NoError(t, err)

	// Optional is nil: in the nested recursion it routes to $unset, but the
	// whole-object replacement already drops it, so it must not surface anywhere.
	entity := &nestedParentOptionalEntity{ID: "p1", Child: &nestedChildWithOptional{Value: "v"}}

	doc, err := m.ConvertToUpdateDocument(t.Context(), entity)
	require.NoError(t, err)

	set, ok := doc["$set"].(bson.M)
	require.True(t, ok, "expected $set document")

	child, ok := set["child"].(bson.M)
	require.True(t, ok, "nested struct pointer must be set as a whole subdocument")
	require.Equal(t, "v", child["value"])
	require.NotContains(t, child, "optional",
		"nil nested field must be absent from the replacement subdocument")

	// No nested $unset must be generated (neither dotted nor object-shaped).
	if unset, ok := doc["$unset"].(bson.M); ok {
		require.NotContains(t, unset, "child")
		require.NotContains(t, unset, "child.optional")
	}
}

// TestConvertToUpdateDocument_NestedMapStruct_WholeObjectReplacement pins the same
// whole-object-replacement contract for struct values inside a map.
func TestConvertToUpdateDocument_NestedMapStruct_WholeObjectReplacement(t *testing.T) {
	t.Parallel()

	m, err := New("testdb")
	require.NoError(t, err)

	entity := &nestedMapParentEntity{
		ID:   "p2",
		Kids: map[string]nestedChildWithOptional{"a": {Value: "x"}},
	}

	doc, err := m.ConvertToUpdateDocument(t.Context(), entity)
	require.NoError(t, err)

	set, ok := doc["$set"].(bson.M)
	require.True(t, ok, "expected $set document")

	kids, ok := set["kids"].(bson.M)
	require.True(t, ok, "map field must be set as a subdocument")

	kidA, ok := kids["a"].(bson.M)
	require.True(t, ok, "map struct value must be set as a whole subdocument")
	require.Equal(t, "x", kidA["value"])
	require.NotContains(t, kidA, "optional",
		"nil nested field must be absent from the replacement subdocument")
}
