// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"sync"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestFreshReadBypassesPreWriteFlight(t *testing.T) {
	t.Parallel()

	for _, perCall := range []bool{false, true} {
		name := "context"
		if perCall {
			name = "per-call settings"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				old := cursorBatchReply("db.items", bson.D{{Key: "name", Value: "old"}})
				updated := cursorBatchReply("db.items", bson.D{{Key: "name", Value: "new"}})
				ack := bson.D{{Key: "ok", Value: 1}, {Key: "n", Value: 1}, {Key: "nModified", Value: 1}}
				m, db, entered, release, finds := gatedRepository(t, old, ack, updated)
				releaseFlight := sync.OnceFunc(func() { close(release) })
				defer releaseFlight()
				coll := db.Collection("items")
				ctx := t.Context()
				filter := bson.D{{Key: "_id", Value: 1}}
				read := func() (flightModel, error) {
					return GetEntity[flightModel, flightModel](ctx, m, coll, filter, FilterEncoding{Registry: bson.NewRegistry()})
				}
				a := beginRead(read)
				<-entered
				result, err := coll.UpdateOne(ctx, filter, bson.M{"$set": bson.M{"name": "new"}})
				require.NoError(t, err)
				require.EqualValues(t, 1, result.ModifiedCount)
				joined := beginRead(read)
				synctest.Wait()
				select {
				case <-joined:
					t.Fatal("ordinary read completed before the original flight")
				default:
				}

				fresh := beginRead(func() (flightModel, error) {
					if perCall {
						return GetEntity[flightModel, flightModel](ctx, m, coll, filter, FilterEncoding{Registry: bson.NewRegistry(), FreshRead: true})
					}
					return GetEntity[flightModel, flightModel](WithFreshRead(ctx), m, coll, filter, FilterEncoding{Registry: bson.NewRegistry()})
				})
				synctest.Wait()
				releaseFlight()

				require.Equal(t, "new", readValue(t, fresh).Name)
				readValue(t, a)
				readValue(t, joined)
				require.EqualValues(t, 2, finds.Load())
			})
		})
	}
}
