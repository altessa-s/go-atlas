// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package pool

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"

	corecontext "github.com/altessa-s/go-atlas/core/context"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// ConnectionPool manages a pool of gRPC client connections with automatic cleanup
// and health monitoring. Connections are partitioned by target address; each target
// gets an independent sub-pool bounded by the configured size.
//
// A ConnectionPool is created in an inactive state via [New]. Call [ConnectionPool.Start]
// to launch the background cleanup goroutine, and use the returned stop function
// (or cancel the context) to shut down. After shutdown, [GetConnection] returns
// [ErrConnectionPoolClosed].
//
// All exported methods are safe for concurrent use.
type ConnectionPool struct {
	opts      *options
	logger    *slog.Logger
	pools     sync.Map // map[string]*targetPool
	connOwner sync.Map // map[*grpc.ClientConn]*targetPool — O(1) lookup for ReturnConnection
	stopped   atomic.Bool
}

// targetPool manages connections for a specific target address.
type targetPool struct {
	pool        *ConnectionPool // back-reference for conn ownership tracking
	target      string
	opts        *options
	logger      *slog.Logger
	connections chan *pooledConnection
	active      sync.Map // map[*grpc.ClientConn]*pooledConnection
	closed      atomic.Bool
}

// pooledConnection wraps a gRPC client connection with pool metadata.
type pooledConnection struct {
	conn     *grpc.ClientConn
	target   string
	created  time.Time
	lastUsed atomic.Int64 // Unix nanoseconds
	inUse    atomic.Bool
}

// New creates a new connection pool with the specified options.
// The pool is inactive until Start is called to begin background cleanup.
//
// Example:
//
//	p := pool.New(pool.WithPoolSize(20))
//	stop, _ := p.Start(ctx)
//	defer stop()
//	conn, _ := p.GetConnection(ctx, "localhost:8080")
//	defer p.ReturnConnection(conn)
func New(opts ...Option) *ConnectionPool {
	options := newOptions(opts...)
	return &ConnectionPool{
		opts:   options,
		logger: options.logger,
	}
}

// Start begins background cleanup of idle connections.
// Returns a stop function that stops cleanup and closes all connections.
//
// The stop function:
//   - Stops the cleanup goroutine and closes all connections
//   - Is idempotent and safe to call multiple times
//   - Blocks until the goroutine exits and cleanup completes
//
// The cleanup can be terminated in two ways:
//   - Canceling the context - triggers cleanup and goroutine exit
//   - Calling stop() - triggers the same cleanup explicitly
//
// Example:
//
//	stop, err := pool.Start(ctx)
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer stop()
func (cp *ConnectionPool) Start(ctx context.Context) (func(), error) {
	if cp.stopped.Load() {
		return nil, ErrConnectionPoolClosed
	}

	cp.logger.InfoContext(ctx, "connection pool started",
		"pool_size", cp.opts.size,
		"max_idle_time", cp.opts.maxIdleTime,
		"cleanup_interval", cp.opts.cleanupInterval)

	stopCh := make(chan struct{})
	done := make(chan struct{})
	var stopOnce sync.Once

	go func() {
		defer close(done)

		ticker := time.NewTicker(cp.opts.cleanupInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				cp.closeAllPools(ctx)
				return
			case <-stopCh:
				cp.closeAllPools(ctx)
				return
			case <-ticker.C:
				cp.cleanup() //nolint:contextcheck // background cleanup uses internal context
			}
		}
	}()

	stop := func() {
		stopOnce.Do(func() {
			cp.stopped.Store(true)
			close(stopCh)
		})
		<-done
	}

	return stop, nil
}

// closeAllPools closes all target pools and their connections.
func (cp *ConnectionPool) closeAllPools(ctx context.Context) {
	cp.stopped.Store(true)
	cp.logger.InfoContext(ctx, "shutting down connection pool")

	cp.pools.Range(func(key, value any) bool {
		tPool, ok := value.(*targetPool)
		if ok {
			tPool.close(ctx)
		}
		return true
	})

	cp.logger.InfoContext(ctx, "connection pool shutdown complete")
}

// GetConnection retrieves a healthy connection for target from the pool.
// If no idle connection is available, a new one is created via the configured
// [ClientFactory] (or the default insecure dialer).
//
// The caller must call [ConnectionPool.ReturnConnection] when done; failing to do so
// leaks the connection. Returns [ErrConnectionPoolClosed] after shutdown.
func (cp *ConnectionPool) GetConnection(ctx context.Context, target string) (*grpc.ClientConn, error) {
	if cp.stopped.Load() {
		return nil, ErrConnectionPoolClosed
	}

	// Get or create target pool
	tPool := cp.getOrCreateTargetPool(target) //nolint:contextcheck // pool creation is context-independent
	return tPool.getConnection(ctx)
}

// ReturnConnection returns a connection to the pool for reuse.
// Unhealthy connections and connections returned after shutdown are closed
// instead of being placed back in the pool. Passing nil is a no-op.
func (cp *ConnectionPool) ReturnConnection(conn *grpc.ClientConn) {
	if conn == nil {
		return
	}

	// O(1) lookup via connOwner map instead of iterating all target pools
	val, ok := cp.connOwner.Load(conn)
	if !ok {
		return
	}
	tPool := val.(*targetPool) //nolint:errcheck // type is guaranteed by store
	tPool.returnConnection(conn)
}

// getOrCreateTargetPool retrieves or creates a target pool for the specified address.
func (cp *ConnectionPool) getOrCreateTargetPool(target string) *targetPool {
	if existing, ok := cp.pools.Load(target); ok {
		if existingPool, poolOK := existing.(*targetPool); poolOK {
			return existingPool
		}
	}

	// Create new target pool
	tPool := &targetPool{
		pool:        cp,
		target:      target,
		opts:        cp.opts,
		logger:      cp.logger.With("target", target),
		connections: make(chan *pooledConnection, cp.opts.size),
	}

	// Try to store the new pool
	if existing, loaded := cp.pools.LoadOrStore(target, tPool); loaded {
		// Another goroutine created the pool first
		if existingPool, ok := existing.(*targetPool); ok {
			return existingPool
		}
		// Fallback to creating new pool if type assertion fails
	}

	//nolint:contextcheck // lazy initialization without request context
	tPool.logger.InfoContext(context.Background(), "created target pool", "pool_size", cp.opts.size)
	return tPool
}

// cleanup performs cleanup of idle connections across all target pools.
func (cp *ConnectionPool) cleanup() {
	cleanedTotal := 0

	cp.pools.Range(func(key, value any) bool {
		tPool, ok := value.(*targetPool)
		if ok {
			cleaned := tPool.cleanup()
			cleanedTotal += cleaned
		}
		return true
	})

	if cleanedTotal > 0 {
		//nolint:contextcheck // background cleanup goroutine has no request context
		cp.logger.DebugContext(context.Background(), "cleaned up idle connections", "count", cleanedTotal)
	}
}

// getConnection retrieves a connection from the target pool.
func (tp *targetPool) getConnection(ctx context.Context) (*grpc.ClientConn, error) {
	if tp.closed.Load() {
		return nil, ErrConnectionPoolClosed
	}

	// Try to get an existing connection
	select {
	case pc := <-tp.connections:
		// Check if connection is still healthy
		if tp.isHealthy(pc) {
			pc.inUse.Store(true)
			pc.lastUsed.Store(time.Now().UnixNano())
			return pc.conn, nil
		}
		// Connection is unhealthy, close it and try to create a new one
		tp.closeConnection(pc)
	default:
		// No connections available, try to create a new one
	}

	// Create new connection
	return tp.createConnection(ctx)
}

// returnConnection returns a connection to the target pool.
func (tp *targetPool) returnConnection(conn *grpc.ClientConn) bool {
	if conn == nil {
		return false
	}

	// Find this connection in active connections
	if value, ok := tp.active.Load(conn); ok {
		pc, pcOK := value.(*pooledConnection)
		if !pcOK {
			return false
		}
		if tp.closed.Load() {
			tp.closeConnection(pc)
			return true
		}
		pc.inUse.Store(false)
		pc.lastUsed.Store(time.Now().UnixNano())

		// Return to pool if there's space and connection is healthy
		if tp.isHealthy(pc) {
			select {
			case tp.connections <- pc:
				return true
			default:
				// Pool is full, close the connection
				tp.closeConnection(pc)
				return true
			}
		} else {
			// Connection is unhealthy, close it
			tp.closeConnection(pc)
			return true
		}
	}

	return false
}

// createConnection establishes a new connection to the target.
func (tp *targetPool) createConnection(ctx context.Context) (*grpc.ClientConn, error) {
	tp.logger.DebugContext(ctx, "establishing new connection")

	// Apply connect timeout if not already set in context
	ctx, cancel := corecontext.ApplyTimeout(ctx, tp.opts.connectTimeout)
	defer cancel()

	// Use custom client factory if provided, otherwise use default
	var conn *grpc.ClientConn
	var err error

	if tp.opts.clientFactory != nil {
		conn, err = tp.opts.clientFactory(ctx, tp.target)
	} else {
		// Default: create connection with insecure credentials
		conn, err = grpc.NewClient(tp.target,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
	}

	if err != nil {
		tp.logger.ErrorContext(ctx, "failed to create connection", "error", err)
		return nil, coreerrs.Wrapf(err, "failed to connect to %s", tp.target)
	}

	pc := &pooledConnection{
		conn:    conn,
		target:  tp.target,
		created: time.Now(),
	}
	pc.inUse.Store(true)
	pc.lastUsed.Store(time.Now().UnixNano())

	// Register in active connections
	tp.active.Store(conn, pc)

	// Register conn→targetPool mapping for O(1) ReturnConnection
	if tp.pool != nil {
		tp.pool.connOwner.Store(conn, tp)
	}

	tp.logger.DebugContext(ctx, "established new connection")
	return conn, nil
}

// isHealthy checks if a pooled connection is healthy and usable.
func (tp *targetPool) isHealthy(pc *pooledConnection) bool {
	if pc == nil || pc.conn == nil {
		return false
	}

	state := pc.conn.GetState()
	return state == connectivity.Ready || state == connectivity.Idle
}

// closeConnection closes a pooled connection and removes it from tracking.
func (tp *targetPool) closeConnection(pc *pooledConnection) {
	if pc == nil || pc.conn == nil {
		return
	}

	tp.active.Delete(pc.conn)

	// Remove conn→targetPool mapping
	if tp.pool != nil {
		tp.pool.connOwner.Delete(pc.conn)
	}

	_ = pc.conn.Close() // #nosec G104 -- error ignored in cleanup path
}

// cleanup removes idle connections from the target pool.
func (tp *targetPool) cleanup() int {
	if tp.closed.Load() {
		return 0
	}

	cleaned := 0
	now := time.Now()
	cutoff := now.Add(-tp.opts.maxIdleTime).UnixNano()

	// Check connections in the pool (bounded to current queue size)
	toCheck := len(tp.connections)
	for range toCheck {
		select {
		case pc := <-tp.connections:
			lastUsed := pc.lastUsed.Load()
			if lastUsed < cutoff || !tp.isHealthy(pc) {
				tp.closeConnection(pc)
				cleaned++
			} else {
				// Connection is still fresh, put it back
				select {
				case tp.connections <- pc:
				default:
					// Pool is full, close this connection
					tp.closeConnection(pc)
					cleaned++
				}
			}
		default:
			return cleaned // No more connections to check
		}
	}
	return cleaned
}

// close shuts down the target pool and closes all connections.
func (tp *targetPool) close(ctx context.Context) {
	if !tp.closed.CompareAndSwap(false, true) {
		return
	}

	tp.logger.InfoContext(ctx, "closing target pool")

	// Close all connections in the pool
	for {
		select {
		case pc := <-tp.connections:
			tp.closeConnection(pc)
		default:
			goto closeActive
		}
	}

closeActive:
	// Close all active connections
	tp.active.Range(func(key, value any) bool {
		pc, ok := value.(*pooledConnection)
		if ok {
			tp.closeConnection(pc)
		}
		return true
	})

	tp.logger.InfoContext(ctx, "target pool closed")
}
