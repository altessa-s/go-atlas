// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo"

	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

func TestNew_WithClient_And_ClientOptions_MutuallyExclusive(t *testing.T) {
	client, err := mongo.Connect(mongoOptions.Client().ApplyURI("mongodb://localhost:27017"))
	require.NoError(t, err, "failed to create test client")

	_, err = New("testdb",
		WithClient(client),
		WithClientOptions(mongoOptions.Client().ApplyURI("mongodb://localhost:27017")),
	)
	require.Error(t, err, "expected error when both WithClient and WithClientOptions are set")
	require.Equal(t, "WithClient and WithClientOptions are mutually exclusive", err.Error())
}

func TestNew_WithClient_SetsClient(t *testing.T) {
	client, err := mongo.Connect(mongoOptions.Client().ApplyURI("mongodb://localhost:27017"))
	require.NoError(t, err, "failed to create test client")

	m, err := New("testdb", WithClient(client))
	require.NoError(t, err)
	require.Equal(t, client, m.config.Client, "expected config.Client to be the provided client")
}

func TestWithClient_Nil_Ignored(t *testing.T) {
	m, err := New("testdb", WithClient(nil))
	require.NoError(t, err)
	require.Nil(t, m.config.Client, "expected config.Client to be nil when WithClient(nil) is used")
}

func TestNew_WithoutClient_BackwardCompatible(t *testing.T) {
	m, err := New("testdb")
	require.NoError(t, err)
	require.Nil(t, m.config.Client, "expected config.Client to be nil by default")
}
