// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

//go:build stress

package pool_test

import (
	"bytes"
	"context"
	"errors"
	"math/rand/v2"
	"runtime"
	"runtime/pprof"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/transport/grpc/client/pool"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
)

var errFactory = errors.New("stress factory failure")

// goroutineDump returns the stacks of every goroutine, for watchdog failures.
func goroutineDump() string {
	var buf bytes.Buffer
	_ = pprof.Lookup("goroutine").WriteTo(&buf, 1) //nolint:errcheck // best-effort diagnostics
	return buf.String()
}

// TestStressPool borrows and returns connections across a few targets with a
// tiny pool size while borrowers cancel, connections turn unhealthy, the
// factory fails, and the pool is stopped under load, cycle after cycle.
// Borrowed connections per target must never exceed the size, no connection
// may be handed to two borrowers at once, stop and the borrowers must finish
// (watchdog), every created connection must end closed, and no goroutine may
// outlive the pools.
//
// Not parallel: the leak check compares the process goroutine count with the
// count before the first pool.
func TestStressPool(t *testing.T) {
	const (
		size      = 2
		borrowers = 12
		watchdog  = 10 * time.Second
	)
	targets := []string{"localhost:1", "localhost:2", "localhost:3"}
	deadline := time.Now().Add(testhelpers.StressDuration(t))
	baseline := runtime.NumGoroutine()

	var created sync.Mutex
	var conns []*grpc.ClientConn
	factory := func(ctx context.Context, target string) (*grpc.ClientConn, error) {
		switch r := rand.IntN(100); { //nolint:gosec // test chaos
		case r < 5:
			return nil, errFactory
		case r < 15:
			select {
			case <-time.After(time.Millisecond):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err == nil {
			created.Lock()
			conns = append(conns, conn)
			created.Unlock()
		}
		return conn, err
	}

	cycles := 0
	var borrows atomic.Int64
	for time.Now().Before(deadline) && !t.Failed() {
		cycles++
		p := pool.New(pool.WithSize(size), pool.WithClientFactory(factory),
			pool.WithCleanupInterval(time.Second), pool.WithMaxIdleTime(time.Second))
		stop, err := p.Start(t.Context())
		require.NoError(t, err)
		bindings := make(map[string]*pool.Binding, len(targets))
		for _, target := range targets {
			b, err := p.Bind(target, factory)
			require.NoError(t, err)
			bindings[target] = b
		}
		unsubscribe, err := p.SubscribeTarget(targets[0], func(connectivity.State) {})
		require.NoError(t, err)

		var (
			mu       sync.Mutex
			borrowed = make(map[string]int)
			holders  = make(map[*grpc.ClientConn]int)
			wg       sync.WaitGroup
		)
		for id := range borrowers {
			wg.Go(func() {
				for {
					target := targets[rand.IntN(len(targets))] //nolint:gosec // test chaos

					timeout := time.Duration(rand.IntN(5000)) * time.Microsecond //nolint:gosec // test chaos
					ctx, cancel := context.WithTimeout(t.Context(), timeout)
					var conn *grpc.ClientConn
					var err error
					if id%3 == 0 {
						conn, err = bindings[target].GetConnection(ctx)
					} else {
						conn, err = p.GetConnection(ctx, target)
					}
					cancel()
					switch {
					case errors.Is(err, pool.ErrConnectionPoolClosed):
						return
					case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled), errors.Is(err, errFactory):
						continue
					case err != nil:
						t.Errorf("GetConnection(%s): unexpected error: %v", target, err)
						return
					}
					borrows.Add(1)
					mu.Lock()
					borrowed[target]++
					if n := borrowed[target]; n > size {
						t.Errorf("%s: %d connections borrowed at once, pool size is %d", target, n, size)
					}
					if other, held := holders[conn]; held {
						t.Errorf("%s: connection handed to borrower %d while borrower %d holds it", target, id, other)
					}
					holders[conn] = id
					mu.Unlock()

					if rand.IntN(10) == 0 { //nolint:gosec // test chaos
						conn.Connect() // nothing listens: the connection turns unhealthy
					}
					time.Sleep(time.Duration(rand.IntN(500)) * time.Microsecond) //nolint:gosec // test chaos

					mu.Lock()
					borrowed[target]--
					delete(holders, conn)
					mu.Unlock()
					p.ReturnConnection(conn)
				}
			})
		}
		wg.Go(func() {
			for range 100 {
				_ = p.CheckHealth(t.Context())
				_ = p.StateForTarget(targets[rand.IntN(len(targets))]) //nolint:gosec // test chaos
				time.Sleep(time.Millisecond)
			}
		})

		time.Sleep(time.Duration(100+rand.IntN(400)) * time.Millisecond) //nolint:gosec // test jitter
		finished := make(chan struct{})
		go func() {
			stop()
			unsubscribe()
			wg.Wait()
			close(finished)
		}()
		select {
		case <-finished:
		case <-time.After(watchdog):
			require.FailNow(t, "deadlock: stop or a borrower did not finish", "cycle %d, goroutines:\n%s", cycles, goroutineDump())
		}
	}
	require.False(t, t.Failed(), "invariant violated during the run")

	created.Lock()
	defer created.Unlock()
	for i, conn := range conns {
		require.Equal(t, connectivity.Shutdown, conn.GetState(), "connection %d was never closed", i)
	}
	ok := false
	for wait := time.Now().Add(5 * time.Second); time.Now().Before(wait); time.Sleep(10 * time.Millisecond) {
		if runtime.NumGoroutine() <= baseline {
			ok = true
			break
		}
	}
	require.True(t, ok, "goroutines leaked: %d now, %d before the first pool\n%s", runtime.NumGoroutine(), baseline, goroutineDump())
	t.Logf("cycles=%d borrows=%d connections=%d", cycles, borrows.Load(), len(conns))
}
