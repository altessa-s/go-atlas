// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver/drivertest"

	"github.com/altessa-s/go-atlas/store/mongo/migrations"
)

type startupGate struct {
	*drivertest.MockDeployment
	entered chan struct{}
	release chan struct{}
	closes  atomic.Int32
}

func (d *startupGate) Connect() error {
	close(d.entered)
	<-d.release
	return d.MockDeployment.Connect()
}

func (d *startupGate) Disconnect(ctx context.Context) error {
	d.closes.Add(1)
	return d.MockDeployment.Disconnect(ctx)
}

func TestCloseDuringConnect(t *testing.T) {
	t.Parallel()

	d := &startupGate{
		MockDeployment: drivertest.NewMockDeployment(
			bson.D{{Key: "ok", Value: 1}},
			bson.D{{Key: "ok", Value: 1}, {Key: "version", Value: "4.0"}, {Key: "versionArray", Value: bson.A{int32(4), int32(0)}}},
		),
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	opts := options.Client()
	opts.Deployment = d
	m, err := New("db", WithClientOptions(opts))
	require.NoError(t, err)
	t.Cleanup(m.Close)
	connected := make(chan error, 1)
	go func() { connected <- m.Connect(t.Context()) }()
	<-d.entered
	unblock := sync.OnceFunc(func() { close(d.release) })
	defer unblock()
	closing, closed := make(chan struct{}), make(chan struct{})
	go func() {
		close(closing)
		m.Close()
		close(closed)
	}()
	<-closing
	select {
	case <-closed:
		t.Error("Close returned before startup completed")
	case <-time.After(50 * time.Millisecond):
	}

	unblock()
	require.Error(t, <-connected)
	<-closed
	require.EqualValues(t, 1, d.closes.Load())
}

// mockStartupOptions lets Connect complete without a live MongoDB or migrations.
func mockStartupOptions() *options.ClientOptions {
	ok := bson.D{{Key: "ok", Value: 1}}
	write := bson.D{{Key: "ok", Value: 1}, {Key: "n", Value: 1}, {Key: "nModified", Value: 1}}
	opts := options.Client()
	opts.Deployment = drivertest.NewMockDeployment(
		ok, // ping
		bson.D{{Key: "ok", Value: 1}, {Key: "version", Value: "8.0"}, {Key: "versionArray", Value: bson.A{int32(8), int32(0)}}},
		write, write, write, // seed, acquire and verify the migration lock
		cursorBatchReply("db.$cmd.listCollections"),
		ok, // create the migration collection
		cursorBatchReply("db.migrations"),
		write, // release the migration lock
		ok,    // endSessions
	)
	return opts
}

func connectMockMongo(t *testing.T, opts ...Option) *Mongo {
	t.Helper()
	m, err := New("db", append(opts, WithClientOptions(mockStartupOptions()), WithMigrationRegistry(&migrations.Registry{}))...)
	require.NoError(t, err)
	t.Cleanup(m.Close)
	require.NoError(t, m.Connect(t.Context()))
	return m
}
