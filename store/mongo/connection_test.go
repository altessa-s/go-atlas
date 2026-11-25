// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/event"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver/drivertest"

	mongotools "github.com/altessa-s/go-atlas/store/mongo"
	kmslocal "github.com/altessa-s/go-atlas/store/mongo/kms/providers/local"
)

type countingDeployment struct {
	*drivertest.MockDeployment
	disconnects int
}

func (d *countingDeployment) Disconnect(ctx context.Context) error {
	d.disconnects++
	return d.MockDeployment.Disconnect(ctx)
}

func TestConnectFailurePreservesClientOwnership(t *testing.T) {
	t.Parallel()
	ok := bson.D{{Key: "ok", Value: 1}}
	oldVersion := bson.D{{Key: "ok", Value: 1}, {Key: "version", Value: "4.0"}, {Key: "versionArray", Value: bson.A{int32(4), int32(0)}}}
	for _, borrowed := range []bool{true, false} {
		name := "owned"
		if borrowed {
			name = "borrowed"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			driverOpts := options.Client()
			deployment := &countingDeployment{MockDeployment: drivertest.NewMockDeployment(ok, oldVersion, ok, oldVersion, ok)}
			driverOpts.Deployment = deployment
			opt := mongotools.WithClientOptions(driverOpts)
			if borrowed {
				client, err := mongo.Connect(driverOpts)
				require.NoError(t, err)
				t.Cleanup(func() { require.NoError(t, client.Disconnect(context.WithoutCancel(t.Context()))) })
				opt = mongotools.WithClient(client)
			}
			m, err := mongotools.New("test", opt)
			require.NoError(t, err)
			t.Cleanup(m.Close)
			require.ErrorIs(t, m.Connect(t.Context()), mongotools.ErrUnsupportedVersion)
			if borrowed {
				require.ErrorIs(t, m.Connect(t.Context()), mongotools.ErrUnsupportedVersion)
				m.Close()
				require.Zero(t, deployment.disconnects)
				require.NoError(t, m.Client().Ping(t.Context(), readpref.Primary()))
			} else {
				m.Close()
				require.Equal(t, 1, deployment.disconnects)
			}
		})
	}
}

func TestConnectCreatesKeyVaultIndex(t *testing.T) {
	t.Parallel()
	const failureCode = 12345
	provider, err := kmslocal.New(kmslocal.WithMasterKey(strings.Repeat("k", kmslocal.RequiredMasterKeyLength)))
	require.NoError(t, err)
	t.Cleanup(provider.Clear)
	var commands []bson.Raw
	opts := options.Client().SetMonitor(&event.CommandMonitor{Started: func(_ context.Context, e *event.CommandStartedEvent) {
		if e.CommandName == "create" || e.CommandName == "createIndexes" {
			commands = append(commands, append(bson.Raw(nil), e.Command...))
		}
	}})
	opts.Deployment = drivertest.NewMockDeployment(
		bson.D{{Key: "ok", Value: 1}},
		bson.D{{Key: "ok", Value: 1}, {Key: "version", Value: "8.0"}, {Key: "versionArray", Value: bson.A{int32(8), int32(0)}}},
		bson.D{{Key: "ok", Value: 0}, {Key: "code", Value: mongotools.MongoErrorCodeCollectionExists}, {Key: "errmsg", Value: "collection exists"}},
		bson.D{{Key: "ok", Value: 0}, {Key: "code", Value: failureCode}, {Key: "errmsg", Value: "index creation failed"}},
	)
	m, err := mongotools.New("app", mongotools.WithClientOptions(opts), mongotools.WithKMS(provider),
		mongotools.WithEncryptionEnabled(), mongotools.WithVaultDatabase("vault"), mongotools.WithVaultCollection("keys"))
	require.NoError(t, err)
	t.Cleanup(m.Close)
	var commandErr mongo.CommandError
	require.ErrorAs(t, m.Connect(t.Context()), &commandErr)
	require.True(t, commandErr.HasErrorCode(failureCode))
	require.Len(t, commands, 2)
	require.Equal(t, "keys", commands[0].Lookup("create").StringValue())
	require.Equal(t, "keys", commands[1].Lookup("createIndexes").StringValue())
	for _, cmd := range commands {
		require.Equal(t, "vault", cmd.Lookup("$db").StringValue())
	}
	index := commands[1].Lookup("indexes").Array().Index(0).Document()
	require.EqualValues(t, 1, index.Lookup("key", "keyAltNames").AsInt64())
	require.True(t, index.Lookup("unique").Boolean())
	require.True(t, index.Lookup("partialFilterExpression", "keyAltNames", "$exists").Boolean())
}
