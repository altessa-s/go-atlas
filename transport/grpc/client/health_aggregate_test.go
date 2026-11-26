// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/health"
	"github.com/altessa-s/go-atlas/transport/grpc/client/pool"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
)

// aggregatePool is a ConnPool whose target state is steered by the test and
// whose subscriber callback is captured to emit per-connection events.
type aggregatePool struct {
	mu     sync.Mutex
	state  connectivity.State
	notify func(connectivity.State)
}

func (p *aggregatePool) Bind(string, pool.ClientFactory) (pool.Binding, error) {
	return aggregateBinding{}, nil
}
func (*aggregatePool) ReturnConnection(*grpc.ClientConn) {}
func (p *aggregatePool) StateForTarget(string) connectivity.State {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state
}
func (p *aggregatePool) SubscribeTarget(_ string, cb func(connectivity.State)) (func(), error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.notify = cb
	return func() {}, nil
}

type aggregateBinding struct{}

func (aggregateBinding) GetConnection(context.Context) (*grpc.ClientConn, error) {
	return grpc.NewClient("passthrough:///aggregate", grpc.WithTransportCredentials(insecure.NewCredentials()))
}

// TestPoolEventPushesTargetAggregate emits a TransientFailure event of one
// connection while the target aggregate stays Ready. The pushed status must
// equal the registered checker's answer, not the single connection's state.
func TestPoolEventPushesTargetAggregate(t *testing.T) {
	t.Parallel()

	coord := health.New()
	t.Cleanup(coord.Close)
	fake := &aggregatePool{state: connectivity.Ready}

	c, err := New(t.Context(), "passthrough:///aggregate",
		WithInsecure(), WithPool(fake), WithHealthCoordinator(coord))
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close(t.Context()) })

	sub, err := coord.Subscribe(t.Context(), DefaultClientHealthServiceName)
	require.NoError(t, err)
	t.Cleanup(sub.Close)
	drainUpdates(sub, 100*time.Millisecond)

	fake.mu.Lock()
	notify := fake.notify
	fake.mu.Unlock()
	require.NotNil(t, notify)

	notify(connectivity.TransientFailure)

	want := c.health.CheckHealth(t.Context())
	require.Equal(t, health.StatusServing, want)
	for _, got := range drainUpdates(sub, 200*time.Millisecond) {
		require.Equal(t, want, got, "push must equal the checker result")
	}
}

// drainUpdates collects the updates delivered within d.
func drainUpdates(sub health.Subscriber, d time.Duration) []health.ServingStatus {
	var got []health.ServingStatus
	timer := time.After(d)
	for {
		select {
		case s := <-sub.Updates():
			got = append(got, s)
		case <-timer:
			return got
		}
	}
}
