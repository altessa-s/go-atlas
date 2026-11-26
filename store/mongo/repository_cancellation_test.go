// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"
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

func gatedRepository(t *testing.T, deduplicate bool, replies ...bson.D) (*Mongo, *mongo.Database, <-chan struct{}, chan<- struct{}, *atomic.Int32) {
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
	coll := dedupTestCollection(t, opts, replies...)
	db := coll.Database()
	mongoOpts := []Option{WithClient(db.Client())}
	if deduplicate {
		mongoOpts = []Option{WithClient(db.Client()), WithReadDeduplication()}
	}
	m, err := New("db", mongoOpts...)
	require.NoError(t, err)
	return m, db, entered, release, &finds
}

func TestGetEntitySendsServerDeadline(t *testing.T) {
	t.Parallel()

	var command bson.Raw
	opts := options.Client().SetMonitor(&event.CommandMonitor{Started: func(_ context.Context, e *event.CommandStartedEvent) {
		if e.CommandName == "find" {
			command = append(bson.Raw(nil), e.Command...)
		}
	}})
	opts.Deployment = drivertest.NewMockDeployment(cursorBatchReply("db.items", bson.D{{Key: "name", Value: "value"}}))
	client, err := mongo.Connect(opts)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Disconnect(context.WithoutCancel(t.Context()))) })
	m, err := New("db", WithClient(client))
	require.NoError(t, err)
	_, err = GetEntity[flightModel, flightModel](t.Context(), m, client.Database("db").Collection("items"), bson.D{}, FilterEncoding{Registry: bson.NewRegistry()})
	require.NoError(t, err)
	maxTime, err := command.LookupErr("maxTimeMS")
	require.NoError(t, err)
	require.Positive(t, maxTime.AsInt64())
	require.LessOrEqual(t, maxTime.AsInt64(), DefaultQueryTimeout.Milliseconds())
}

func TestReadFlightCancellationDuringDecode(t *testing.T) {
	t.Parallel()

	type model struct{ Name string }
	// Use a wall-clock timer because a broken implementation can block on a mutex.
	timedOut := make(chan struct{})
	timer := time.AfterFunc(time.Second, func() { close(timedOut) })
	defer timer.Stop()
	synctest.Test(t, func(t *testing.T) {
		doc := bson.D{{Key: "name", Value: "value"}}
		m, db, entered, release, finds := gatedRepository(t, true, cursorBatchReply("db.items", doc, doc))
		decoderStarted, decoderRelease := make(chan struct{}, 4), make(chan struct{})
		unblock := sync.OnceFunc(func() { close(decoderRelease) })
		defer unblock()
		var calls atomic.Int32
		reg := bson.NewRegistry()
		reg.RegisterTypeDecoder(reflect.TypeFor[model](), bson.ValueDecoderFunc(func(_ bson.DecodeContext, vr bson.ValueReader, value reflect.Value) error {
			calls.Add(1)
			decoderStarted <- struct{}{}
			<-decoderRelease
			dr, err := vr.ReadDocument()
			if err != nil {
				return err
			}
			_, field, err := dr.ReadElement()
			if err != nil {
				return err
			}
			name, err := field.ReadString()
			if err != nil {
				return err
			}
			value.Set(reflect.ValueOf(model{Name: name}))
			return nil
		}))
		coll := db.Collection("items", options.Collection().SetRegistry(reg))
		read := func(ctx context.Context) ([]model, error) {
			return GetEntities[model, model](ctx, m, coll, bson.D{}, FilterEncoding{Registry: reg})
		}
		a := beginRead(func() ([]model, error) { return read(t.Context()) })
		<-entered
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		b := beginRead(func() ([]model, error) { return read(ctx) })
		synctest.Wait()
		close(release)
		for range 2 {
			select {
			case <-decoderStarted:
			case <-timedOut:
				t.Error("both callers must decode independently")
				return
			}
		}
		cancel()
		select {
		case result := <-b:
			require.ErrorIs(t, result.err, context.Canceled)
		case <-timedOut:
			t.Error("canceled reader waited for the blocked decoder")
		}
		unblock()
		require.Equal(t, []model{{Name: "value"}, {Name: "value"}}, readValue(t, a))
		synctest.Wait()
		require.EqualValues(t, 1, finds.Load())
		require.LessOrEqual(t, calls.Load(), int32(3), "canceled reader must skip the remaining document")
	})
}

func TestRepositoryEmptyResults(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name     string
		settings []FilterEncoding
	}{
		{name: "borrowed"},
		{name: "shared", settings: []FilterEncoding{{Registry: bson.NewRegistry()}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			empty := cursorBatchReply("db.items")
			coll := dedupTestCollection(t, options.Client(), empty, empty)
			m, err := New("db", WithClient(coll.Database().Client()), WithReadDeduplication())
			require.NoError(t, err)
			_, err = GetEntity[flightModel, flightModel](t.Context(), m, coll, bson.D{}, tt.settings...)
			require.Equal(t, mongo.ErrNoDocuments, err)

			got, err := GetEntities[flightModel, flightModel](t.Context(), m, coll, bson.D{}, tt.settings...)
			require.NoError(t, err)
			require.Equal(t, []flightModel{}, got)
		})
	}
}
