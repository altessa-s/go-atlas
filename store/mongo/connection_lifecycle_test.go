// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"
	"runtime"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver/drivertest"
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
		MockDeployment: drivertest.NewMockDeployment(),
		entered:        make(chan struct{}),
		release:        make(chan struct{}),
	}
	opts := options.Client()
	opts.Deployment = d
	m, err := New("db", WithClientOptions(opts))
	require.NoError(t, err)
	t.Cleanup(m.Close)
	connected := make(chan error, 1)
	go func() { connected <- m.Connect(t.Context()) }()
	<-d.entered
	closed := make(chan struct{})
	go func() {
		m.Close()
		close(closed)
	}()
	for m.conn.closing.Load() == 0 {
		select {
		case <-closed:
			close(d.release)
			t.Fatal("Close returned before startup completed")
		default:
			runtime.Gosched()
		}
	}
	close(d.release)
	require.ErrorIs(t, <-connected, mongo.ErrClientDisconnected)
	<-closed
	require.EqualValues(t, 1, d.closes.Load())
}
