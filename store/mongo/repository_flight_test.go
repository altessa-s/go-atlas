// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"testing"
	"testing/synctest"

	"cloud.google.com/go/civil"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"

	"github.com/altessa-s/go-atlas/store/mongo/bsoncodec"
)

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
						return GetEntities[flightModel, *flightModel](t.Context(), m, coll, bson.D{}, FilterEncoding{Registry: bson.NewRegistry()})
					}
					entity, err := GetEntity[flightModel, *flightModel](t.Context(), m, coll, bson.D{}, FilterEncoding{Registry: bson.NewRegistry()})
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
		reg := bsoncodec.NewRegistry()
		coll := db.Collection("items", options.Collection().SetRegistry(reg))
		encoding := FilterEncoding{Registry: reg, BSONOptions: &options.BSONOptions{UseJSONStructTags: true}}
		a := beginRead(func() (entityA, error) {
			return GetEntity[model, entityA](t.Context(), m, coll, bson.D{}, encoding)
		})
		<-entered
		b := beginRead(func() (entityB, error) {
			return GetEntity[model, entityB](t.Context(), m, coll, bson.D{}, encoding)
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
			return GetEntity[flightModel, flightModel](t.Context(), m, secondary, bson.D{}, FilterEncoding{Registry: bson.NewRegistry()})
		})
		<-entered
		b := beginRead(func() (flightModel, error) {
			return GetEntity[flightModel, flightModel](t.Context(), m, primary, bson.D{}, FilterEncoding{Registry: bson.NewRegistry()})
		})
		synctest.Wait()
		close(release)
		readValue(t, a)
		readValue(t, b)
		require.EqualValues(t, 2, finds.Load())
	})
}
