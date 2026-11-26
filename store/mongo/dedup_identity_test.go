// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver/drivertest"
)

type dedupTenantKey struct{}

func dedupTenant(ctx context.Context) string {
	id, _ := ctx.Value(dedupTenantKey{}).(string)
	return id
}

func TestDeduplicationIdentity(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name     string
		identity DeduplicationIdentityFunc
		first    string
		second   string
		finds    int32
	}{
		{name: "different tenants", identity: dedupTenant, first: "A", second: "B", finds: 2},
		{name: "same tenant", identity: dedupTenant, first: "A", second: "A", finds: 1},
		{name: "nil identity", first: "A", second: "B", finds: 1},
		{name: "empty identity", identity: func(context.Context) string { return "" }, first: "A", second: "B", finds: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				first := cursorBatchReply("db.items", bson.D{{Key: "name", Value: "first"}})
				second := cursorBatchReply("db.items", bson.D{{Key: "name", Value: "second"}})
				_, db, entered, release, finds := gatedRepository(t, false, first, second)
				unblock := sync.OnceFunc(func() { close(release) })
				defer unblock()
				m, err := New("db", WithClient(db.Client()), WithReadDeduplication(), WithDeduplicationIdentity(tt.identity))
				require.NoError(t, err)
				coll := db.Collection("items")
				encoding := FilterEncoding{Registry: bson.NewRegistry()}
				read := func(tenant string) (flightModel, error) {
					ctx := context.WithValue(t.Context(), dedupTenantKey{}, tenant)
					return GetEntity[flightModel, flightModel](ctx, m, coll, bson.D{}, encoding)
				}
				a := beginRead(func() (flightModel, error) { return read(tt.first) })
				<-entered
				b := beginRead(func() (flightModel, error) { return read(tt.second) })
				synctest.Wait()
				unblock()

				require.Equal(t, "first", readValue(t, a).Name)
				want := "first"
				if tt.finds == 2 {
					want = "second"
				}
				require.Equal(t, want, readValue(t, b).Name)
				require.Equal(t, tt.finds, finds.Load())
			})
		})
	}
}

func dedupTestCollection(t *testing.T, opts *options.ClientOptions, replies ...bson.D) *mongo.Collection {
	t.Helper()
	opts.Deployment = drivertest.NewMockDeployment(replies...)
	client, err := mongo.Connect(opts)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Disconnect(context.WithoutCancel(t.Context()))) })
	return client.Database("db").Collection("items")
}
