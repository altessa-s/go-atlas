// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"cloud.google.com/go/civil"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/event"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver/drivertest"

	"github.com/altessa-s/go-atlas/store/mongo/bsoncodec"
)

type flightModel struct {
	Name string         `bson:"name"`
	Tags map[string]int `bson:"tags"`
}

type flightRead[T any] struct {
	value T
	err   error
}

func beginRead[T any](read func() (T, error)) <-chan flightRead[T] {
	done := make(chan flightRead[T], 1)
	go func() {
		value, err := read()
		done <- flightRead[T]{value, err}
	}()
	return done
}

func readValue[T any](t *testing.T, read <-chan flightRead[T]) T {
	t.Helper()
	result := <-read
	require.NoError(t, result.err)
	return result.value
}

func gatedRepository(t *testing.T, replies ...bson.D) (*Mongo, *mongo.Database, <-chan struct{}, chan<- struct{}, *atomic.Int32) {
	t.Helper()
	entered, release := make(chan struct{}), make(chan struct{})
	var finds atomic.Int32
	opts := options.Client().SetBSONOptions(&options.BSONOptions{UseJSONStructTags: true})
	opts.SetMonitor(&event.CommandMonitor{Succeeded: func(_ context.Context, e *event.CommandSucceededEvent) {
		if e.CommandName == "find" && finds.Add(1) == 1 {
			close(entered)
			<-release
		}
	}})
	opts.Deployment = drivertest.NewMockDeployment(replies...)
	client, err := mongo.Connect(opts)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Disconnect(context.WithoutCancel(t.Context()))) })
	m, err := New("db", WithClient(client))
	require.NoError(t, err)
	return m, client.Database("db"), entered, release, &finds
}

func TestReadFlightResultsAreIndependent(t *testing.T) {
	t.Parallel()

	for _, many := range []bool{false, true} {
		name := "GetEntity"
		if many {
			name = "GetEntities"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				doc := bson.D{{Key: "name", Value: "original"}, {Key: "tags", Value: bson.M{"old": 1}}}
				m, db, entered, release, finds := gatedRepository(t, cursorBatchReply("db.items", doc))
				coll := db.Collection("items")
				read := func() ([]*flightModel, error) {
					if many {
						return GetEntities[flightModel, *flightModel](t.Context(), m, coll, bson.D{})
					}
					entity, err := GetEntity[flightModel, *flightModel](t.Context(), m, coll, bson.D{})
					return []*flightModel{entity}, err
				}
				reads := []<-chan flightRead[[]*flightModel]{beginRead(read)}
				<-entered
				for range 3 {
					reads = append(reads, beginRead(read))
				}
				synctest.Wait()
				close(release)

				var results [][]*flightModel
				for _, pending := range reads {
					results = append(results, readValue(t, pending))
				}
				require.EqualValues(t, 1, finds.Load())
				for _, result := range results[1:] {
					require.NotSame(t, results[0][0], result[0])
				}
				var mutations []<-chan flightRead[int]
				for _, result := range results {
					mutations = append(mutations, beginRead(func() (int, error) {
						item := result[0]
						item.Name = "changed"
						item.Tags["new"] = 2
						result[0] = nil
						return 0, nil
					}))
				}
				for _, done := range mutations {
					readValue(t, done)
				}
			})
		})
	}
}

func TestReadFlightConvertsPerCallerWithCollectionCodec(t *testing.T) {
	t.Parallel()

	type model struct {
		Date civil.Date `json:"day"`
	}
	type entityA struct{ Date civil.Date }
	type entityB struct{ Date civil.Date }

	synctest.Test(t, func(t *testing.T) {
		doc := bson.D{{Key: "day", Value: "2024-01-01"}}
		m, db, entered, release, finds := gatedRepository(t, cursorBatchReply("db.items", doc))
		coll := db.Collection("items", options.Collection().SetRegistry(bsoncodec.NewRegistry()))
		a := beginRead(func() (entityA, error) {
			return GetEntity[model, entityA](t.Context(), m, coll, bson.D{})
		})
		<-entered
		b := beginRead(func() (entityB, error) {
			return GetEntity[model, entityB](t.Context(), m, coll, bson.D{})
		})
		synctest.Wait()
		close(release)
		want := civil.Date{Year: 2024, Month: 1, Day: 1}
		require.Equal(t, want, readValue(t, a).Date)
		require.Equal(t, want, readValue(t, b).Date)
		require.EqualValues(t, 1, finds.Load())
	})
}

func TestReadFlightSeparatesCollectionHandles(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		doc := bson.D{{Key: "name", Value: "value"}}
		m, db, entered, release, finds := gatedRepository(t, cursorBatchReply("db.items", doc), cursorBatchReply("db.items", doc))
		secondary := db.Collection("items", options.Collection().SetReadPreference(readpref.Secondary()))
		primary := db.Collection("items", options.Collection().SetReadPreference(readpref.Primary()))
		a := beginRead(func() (flightModel, error) {
			return GetEntity[flightModel, flightModel](t.Context(), m, secondary, bson.D{})
		})
		<-entered
		b := beginRead(func() (flightModel, error) {
			return GetEntity[flightModel, flightModel](t.Context(), m, primary, bson.D{})
		})
		synctest.Wait()
		close(release)
		readValue(t, a)
		readValue(t, b)
		require.EqualValues(t, 2, finds.Load())
	})
}

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
					return GetEntity[flightModel, flightModel](ctx, m, coll, filter)
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
						return GetEntity[flightModel, flightModel](ctx, m, coll, filter, FilterEncoding{FreshRead: true})
					}
					return GetEntity[flightModel, flightModel](WithFreshRead(ctx), m, coll, filter)
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
