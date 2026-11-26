// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
	"context"
	"sync"
	"testing"

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

// recordingReporter is a synchronous ScopedReporter that records every status
// the client pushes.
type recordingReporter struct {
	mu   sync.Mutex
	sent []health.ServingStatus
}

func (r *recordingReporter) RegisterScopedService(string, health.Checker) (func(health.ServingStatus), func()) {
	return func(s health.ServingStatus) {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.sent = append(r.sent, s)
	}, func() {}
}

func (r *recordingReporter) last(t *testing.T) health.ServingStatus {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	require.NotEmpty(t, r.sent, "no status was pushed")
	return r.sent[len(r.sent)-1]
}

func (r *recordingReporter) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.sent)
}

// TestPoolEventPushesTargetAggregate emits a TransientFailure event of one
// connection while the target aggregate stays Ready. The pushed status must
// be the aggregate the registered checker reports, not the single
// connection's state; and when the aggregate itself degrades the push follows.
func TestPoolEventPushesTargetAggregate(t *testing.T) {
	t.Parallel()

	rec := &recordingReporter{}
	fake := &aggregatePool{state: connectivity.Ready}

	c, err := New(t.Context(), "passthrough:///aggregate",
		WithInsecure(), WithPool(fake), WithScopedHealthReporter(rec))
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close(t.Context()) })

	fake.mu.Lock()
	notify := fake.notify
	fake.mu.Unlock()
	require.NotNil(t, notify)

	before := rec.count()
	notify(connectivity.TransientFailure)
	require.Greater(t, rec.count(), before, "the event pushed nothing")
	require.Equal(t, health.StatusServing, rec.last(t), "push must equal the checker result")
	require.Equal(t, health.StatusServing, c.health.CheckHealth(t.Context()))

	fake.mu.Lock()
	fake.state = connectivity.TransientFailure
	fake.mu.Unlock()
	notify(connectivity.TransientFailure)
	require.Equal(t, health.StatusDegraded, rec.last(t))
	require.Equal(t, health.StatusDegraded, c.health.CheckHealth(t.Context()))
}
