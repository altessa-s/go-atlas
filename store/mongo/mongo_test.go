// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/event"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver/drivertest"

	"github.com/altessa-s/go-atlas/store/mongo/migrations"

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
	require.Equal(t, client, m.conn.externalClient)
}

func TestWithClient_Nil_Ignored(t *testing.T) {
	m, err := New("testdb", WithClient(nil))
	require.NoError(t, err)
	require.Nil(t, m.conn.externalClient)
}

func TestNew_WithoutClient_BackwardCompatible(t *testing.T) {
	m, err := New("testdb")
	require.NoError(t, err)
	require.Nil(t, m.conn.externalClient)
}

func TestNew_ClientRegistryIsolatesStructTags(t *testing.T) {
	t.Parallel()
	type document struct {
		FieldName string `json:"json_name"`
	}
	for _, tc := range []struct {
		name      string
		jsonFirst bool
	}{{"json first", true}, {"bson first", false}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for _, useJSON := range []bool{tc.jsonFirst, !tc.jsonFirst} {
				var written bson.Raw
				opts := mongoOptions.Client().SetBSONOptions(&mongoOptions.BSONOptions{UseJSONStructTags: useJSON}).
					SetMonitor(&event.CommandMonitor{Started: func(_ context.Context, evt *event.CommandStartedEvent) {
						if evt.CommandName == "insert" {
							written = append(bson.Raw(nil), evt.Command.Lookup("documents").Array().Index(0).Document()...)
						}
					}})
				opts.Deployment = drivertest.NewMockDeployment(bson.D{{Key: "ok", Value: 1}, {Key: "n", Value: 1}})
				m, err := New("test", WithClientOptions(opts))
				require.NoError(t, err)
				require.Same(t, m.encryption.registry, m.conn.clientOptions.Registry)
				client, err := mongo.Connect(m.conn.clientOptions)
				require.NoError(t, err)
				t.Cleanup(func() { require.NoError(t, client.Disconnect(context.WithoutCancel(t.Context()))) })
				_, err = client.Database("test").Collection("docs").InsertOne(t.Context(), document{FieldName: "value"})
				require.NoError(t, err)
				want := "fieldname"
				if useJSON {
					want = "json_name"
				}
				v, err := written.LookupErr(want)
				require.NoError(t, err)
				require.Equal(t, "value", v.StringValue())
			}
		})
	}
}

func TestConnectUsesMigrationRegistry(t *testing.T) {
	t.Parallel()
	var r migrations.Registry
	stop := errors.New("selected migration ran")
	require.NoError(t, r.Register(1, "private", func(context.Context, *mongo.Database) error { return stop }, nil))
	ok := bson.D{{Key: "ok", Value: 1}, {Key: "n", Value: 1}}
	cursor := func(namespace string, batch bson.A) bson.D {
		return bson.D{{Key: "ok", Value: 1}, {Key: "cursor", Value: bson.M{"id": int64(0), "ns": namespace, "firstBatch": batch}}}
	}
	opts := mongoOptions.Client()
	opts.Deployment = drivertest.NewMockDeployment(
		ok, bson.D{{Key: "ok", Value: 1}, {Key: "version", Value: "8.0"}, {Key: "versionArray", Value: bson.A{int32(8), int32(0), int32(0), int32(0)}}},
		ok, ok, // Seed and acquire the migration lease.
		cursor("test.$cmd.listCollections", bson.A{bson.M{"name": "migrations"}}),
		cursor("test.migrations", bson.A{}), // The version the run starts from.
		cursor("test.$cmd.listCollections", bson.A{bson.M{"name": "migrations"}}),
		cursor("test.migrations", bson.A{}), ok, // The version Up reads, then release the lease.
	)
	m, err := New("test", WithClientOptions(opts), WithMigrationRegistry(&r))
	require.NoError(t, err)
	t.Cleanup(m.Close)
	require.ErrorIs(t, m.Connect(t.Context()), stop)
}
