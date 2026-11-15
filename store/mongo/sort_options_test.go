// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestSortOptionsNamedTypes(t *testing.T) {
	t.Parallel()
	type sortText string
	type sortPointer *string
	value := "name,-age"
	expected := bson.D{{Key: "name", Value: 1}, {Key: "age", Value: -1}}

	list := defaultListOptions()
	WithListSort(sortText(value))(list)
	require.Equal(t, expected, list.sort)
	WithListSort(sortPointer(&value))(list)
	require.Equal(t, expected, list.sort)

	cursor := defaultListCursorOptions()
	WithListCursorSort(sortText(value))(cursor)
	require.Equal(t, expected, cursor.sort)
	WithListCursorSort(sortPointer(&value))(cursor)
	require.Equal(t, expected, cursor.sort)
}
