// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func TestIsErrorCollectionExists(t *testing.T) {
	t.Parallel()

	exists := mongo.CommandError{Code: MongoErrorCodeCollectionExists, Name: "NamespaceExists"}
	require.True(t, IsErrorCollectionExists(exists))
	require.True(t, IsErrorCollectionExists(fmt.Errorf("create: %w", exists)), "wrapped")
	require.False(t, IsErrorCollectionExists(mongo.CommandError{Code: MongoErrorCodeNamespaceNotFound}))
	require.False(t, IsErrorCollectionExists(errors.New("random error")))
	require.False(t, IsErrorCollectionExists(nil))
}
