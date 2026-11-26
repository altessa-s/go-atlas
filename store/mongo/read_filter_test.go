// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"os"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/event"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver/drivertest"

	"github.com/altessa-s/go-aip/pagination/pagetoken"
	"github.com/altessa-s/go-mongox/bsonfilter"
)

type readDocument struct{ Name string }

func mockReadCollection(t testing.TB, started func(context.Context, *event.CommandStartedEvent), batches ...bson.A) *mongo.Collection {
	t.Helper()
	opts := options.Client().SetMonitor(&event.CommandMonitor{Started: started})
	deployment := drivertest.NewMockDeployment()
	for _, batch := range batches {
		deployment.AddResponses(bson.D{
			{Key: "ok", Value: 1},
			{Key: "cursor", Value: bson.M{"id": int64(0), "ns": "testdb.items", "firstBatch": batch}},
		})
	}
	opts.Deployment = deployment
	c, err := mongo.Connect(opts)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Disconnect(context.WithoutCancel(t.Context())) })
	return c.Database("testdb").Collection("items")
}

func TestPreparedReadCommands(t *testing.T) {
	t.Parallel()
	type scalar struct{ value int32 }
	for _, path := range []string{"GetEntity", "GetEntities", "GetEntitiesSeq", "List", "ListCursor", "ListCursorSeq"} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			reg := bson.NewRegistry()
			var encodes atomic.Int32
			typ := reflect.TypeFor[scalar]()
			reg.RegisterTypeEncoder(typ, bson.ValueEncoderFunc(func(_ bson.EncodeContext, w bson.ValueWriter, v reflect.Value) error {
				encodes.Add(1)
				return w.WriteInt32(v.Interface().(scalar).value)
			}))
			settings := FilterEncoding{Registry: reg, BSONOptions: &options.BSONOptions{IntMinSize: true}}
			filter := bson.M{"z": int64(3), "a": scalar{7}}
			_, err := bsonfilter.Prepare(t.Context(), filter, reg, settings.BSONOptions)
			require.ErrorIs(t, err, bsonfilter.ErrValueType)
			bsonfilter.AdmitScalar(reg, typ)
			want, err := bsonfilter.Prepare(t.Context(), filter, reg, settings.BSONOptions)
			require.NoError(t, err)
			require.Equal(t, bson.TypeInt32, want.Lookup("z").Type)
			encodes.Store(0)
			var cmd bson.Raw
			coll := mockReadCollection(t, func(_ context.Context, e *event.CommandStartedEvent) {
				if e.CommandName == "aggregate" || e.CommandName == "find" {
					cmd = append(bson.Raw(nil), e.Command...)
				}
			}, bson.A{bson.M{"name": "alice"}})
			m, err := New("testdb")
			require.NoError(t, err)
			switch path {
			case "GetEntity":
				_, err = GetEntity[readDocument, readDocument](t.Context(), m, coll, filter, settings)
			case "GetEntities":
				_, err = GetEntities[readDocument, readDocument](t.Context(), m, coll, filter, settings)
			case "GetEntitiesSeq":
				seq := GetEntitiesSeq[readDocument, readDocument](t.Context(), m, coll, filter, settings)
				require.Zero(t, encodes.Load())
				for _, err = range seq {
					require.NoError(t, err)
				}
			case "List":
				_, err = List[bson.M](t.Context(), coll, WithListFilter(filter), WithListFilterEncoding(settings), WithListTotal(true))
			case "ListCursor":
				_, err = ListCursor[bson.M](t.Context(), coll, WithListCursorFilter(filter), WithListCursorFilterEncoding(settings), WithListCursorTotal(true))
			case "ListCursorSeq":
				for _, err = range ListCursorSeq[bson.M](t.Context(), coll, WithListCursorFilter(filter), WithListCursorFilterEncoding(settings), WithListCursorTotal(true)) {
					require.NoError(t, err)
				}
			}
			require.NoError(t, err)
			require.EqualValues(t, 1, encodes.Load())
			if filterDoc, ok := cmd.Lookup("filter").DocumentOK(); ok {
				require.Equal(t, want, filterDoc)
				return
			}
			stages, err := cmd.Lookup("pipeline").Array().Values()
			require.NoError(t, err)
			require.Equal(t, want, stages[0].Document().Lookup("$match").Document())
			if path == "List" {
				return
			}
			for _, stage := range stages {
				if union, ok := stage.Document().Lookup("$unionWith").DocumentOK(); ok {
					require.Equal(t, want, union.Lookup("pipeline").Array().Index(0).Document().Lookup("$match").Document())
					return
				}
			}
			t.Fatal("missing count pipeline")
		})
	}
}

// The fixture was issued by ad4ba63c before the BSON binding change. It
// contains a signed ListCursor token, its stateful counterpart and the wire
// filter. The fixed clock keeps the historical signature within its TTL.
func TestLegacyListCursorFixtures(t *testing.T) {
	t.Parallel()
	var fixture struct {
		Signed, Key, Filter string
		Record              CursorMetadata
	}
	data, err := os.ReadFile("testdata/listcursor/legacy.json")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &fixture))
	raw, err := hex.DecodeString(fixture.Filter)
	require.NoError(t, err)
	for _, stored := range []bool{false, true} {
		var cmd bson.Raw
		coll := mockReadCollection(t, func(_ context.Context, e *event.CommandStartedEvent) {
			if e.CommandName == "aggregate" {
				cmd = append(bson.Raw(nil), e.Command...)
			}
		}, bson.A{})
		req := signedRequest(newCursorSigner(t, 1, pagetoken.WithClock(func() time.Time { return time.Unix(1700000001, 0) })), "tenant-A")
		token := fixture.Signed
		if stored {
			storage := newFakeCursorStorage()
			require.NoError(t, storage.Store(t.Context(), fixture.Key, &fixture.Record))
			req = append(req, WithListCursorStorage(storage))
			token = fixture.Key
		}
		req = append(req, WithListCursorCursor(token), WithListCursorFilter(bson.Raw(raw)))
		_, err = ListCursor[bson.M](t.Context(), coll, req...)
		require.NoError(t, err)
		match := cmd.Lookup("pipeline").Array().Index(0).Document().Lookup("$match", "$and").Array()
		require.Equal(t, bson.Raw(raw), match.Index(0).Document())
		_, err = ListCursor[bson.M](t.Context(), coll, append(req, WithListCursorFilter(bson.M{"status": "closed"}))...)
		require.ErrorIs(t, err, ErrCursorFilterMismatch)
	}
}

func TestPreparedEntityDeduplication(t *testing.T) {
	t.Parallel()
	for _, many := range []bool{false, true} {
		synctest.Test(t, func(t *testing.T) {
			calls := 0
			release := make(chan struct{})
			unblock := sync.OnceFunc(func() { close(release) })
			defer unblock()
			coll := mockReadCollection(t, func(_ context.Context, e *event.CommandStartedEvent) {
				if e.CommandName == "find" {
					calls++
					<-release
				}
			}, bson.A{bson.M{"name": "alice"}})
			m, err := New("testdb")
			require.NoError(t, err)
			encoding := FilterEncoding{Registry: bson.NewRegistry()}
			result := make(chan error, 2)
			read := func(filter any) {
				if many {
					_, err := GetEntities[readDocument, readDocument](t.Context(), m, coll, filter, encoding)
					result <- err
					return
				}
				_, err := GetEntity[readDocument, readDocument](t.Context(), m, coll, filter, encoding)
				result <- err
			}
			go read(bson.M{"b": 2, "a": 1})
			synctest.Wait()
			go read(bson.D{{Key: "a", Value: 1}, {Key: "b", Value: 2}})
			synctest.Wait()
			require.Equal(t, 1, calls)
			unblock()
			require.NoError(t, <-result)
			require.NoError(t, <-result)
		})
	}
}

func TestReadsRejectStructFilters(t *testing.T) {
	t.Parallel()
	for _, doc := range []any{struct{ Count int }{3}, bson.M{"value": struct{ Count int }{3}}} {
		_, err := List[bson.M](t.Context(), nil, WithListFilter(doc))
		require.ErrorIs(t, err, bsonfilter.ErrValueType)
		_, err = ListCursor[bson.M](t.Context(), nil, WithListCursorFilter(doc))
		require.ErrorIs(t, err, bsonfilter.ErrValueType)
		_, err = GetEntity[readDocument, readDocument](t.Context(), nil, nil, doc)
		require.ErrorIs(t, err, bsonfilter.ErrValueType)
		_, err = GetEntities[readDocument, readDocument](t.Context(), nil, nil, doc)
		require.ErrorIs(t, err, bsonfilter.ErrValueType)
		for _, err = range GetEntitiesSeq[readDocument, readDocument](t.Context(), nil, nil, doc) {
			require.ErrorIs(t, err, bsonfilter.ErrValueType)
		}
		for _, err = range ListCursorSeq[bson.M](t.Context(), nil, WithListCursorFilter(doc)) {
			require.ErrorIs(t, err, bsonfilter.ErrValueType)
		}
	}
}

func TestListCursorBindsBSONType(t *testing.T) {
	t.Parallel()
	coll := newDetachedCollection(t)
	for _, stored := range []bool{false, true} {
		req := []ListCursorOption{WithListCursorSort("cursor_id"), WithListCursorFilter(bson.M{"n": int32(3)}), WithListCursorSigner(newCursorSigner(t, 1))}
		if stored {
			req = append(req, WithListCursorStorage(newFakeCursorStorage()))
		}
		opts, err := prepareListCursorOptions(t.Context(), coll, req)
		require.NoError(t, err)
		collection := cursorCollectionDigest(coll)
		token, err := opts.nextCursorToken(t.Context(), bson.NewObjectID().Hex(), nil, collection)
		require.NoError(t, err)
		// JSON hashes are identical for these two values. BSON hashes must differ.
		req = append(req, WithListCursorCursor(token), WithListCursorFilter(bson.M{"n": int64(3)}))
		_, err = ListCursor[bson.M](t.Context(), coll, req...)
		require.ErrorIs(t, err, ErrCursorFilterMismatch)
		req = append(req, WithListCursorFilterEncoding(FilterEncoding{BSONOptions: &options.BSONOptions{IntMinSize: true}}))
		opts, err = prepareListCursorOptions(t.Context(), coll, req)
		require.NoError(t, err)
		require.NoError(t, resolveCursorOptions(t.Context(), opts, collection))
	}
}
