// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package pool

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
)

func (tp *targetPool) isRetired() bool {
	tp.mu.Lock()
	defer tp.mu.Unlock()
	return tp.retired
}

// TestBorrowerRetriesAfterPoolRetirement pauses a borrower between the lookup
// of its target pool and the borrow while cleanup retires that pool. The
// borrower must not use the retired pool: it creates a fresh one, and return
// and shutdown account for the connection by that pool.
func TestBorrowerRetriesAfterPoolRetirement(t *testing.T) {
	t.Parallel()

	p := New(WithClientFactory(func(_ context.Context, target string) (*grpc.ClientConn, error) {
		return grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}))
	stop, err := p.Start(t.Context())
	require.NoError(t, err)
	t.Cleanup(stop)

	looked := make(chan *targetPool)
	resume := make(chan struct{})
	var armed atomic.Bool
	armed.Store(true)
	p.afterLookup = func(tp *targetPool) {
		if armed.CompareAndSwap(true, false) {
			looked <- tp
			<-resume
		}
	}

	type result struct {
		conn *grpc.ClientConn
		err  error
	}
	res := make(chan result, 1)
	go func() {
		conn, err := p.GetConnection(t.Context(), "127.0.0.1:1")
		res <- result{conn, err}
	}()

	old := <-looked
	p.cleanup()
	require.True(t, old.isRetired(), "empty pool must be retired by cleanup")
	close(resume)

	r := <-res
	require.NoError(t, r.err)
	owner, ok := p.connOwner.Load(r.conn)
	require.True(t, ok)
	require.NotSame(t, old, owner, "borrowed from a retired pool")
	current, ok := p.pools.Load("127.0.0.1:1")
	require.True(t, ok)
	require.Same(t, current, owner)

	p.ReturnConnection(r.conn)
	stop()
	require.Equal(t, connectivity.Shutdown, r.conn.GetState(), "shutdown must close the connection")
}
