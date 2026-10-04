// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package pool

import (
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/altessa-s/go-atlas/observability/health"
	"github.com/altessa-s/go-atlas/observability/metrics"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"

	corecontext "github.com/altessa-s/go-atlas/core/context"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// closeReason identifies why a connection was closed.
type closeReason string

const (
	closeReasonIdle      closeReason = "idle"
	closeReasonUnhealthy closeReason = "unhealthy"
	closeReasonPoolFull  closeReason = "pool_full"
	closeReasonShutdown  closeReason = "shutdown"
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
	opts        *options
	logger      *slog.Logger
	metrics     *poolMetrics
	pools       sync.Map // map[string]*targetPool
	connOwner   sync.Map // map[*grpc.ClientConn]*targetPool — O(1) lookup for ReturnConnection
	stopped     atomic.Bool
	lifecycleMu sync.Mutex
	started     bool
	stopOnce    sync.Once
	done        chan struct{}
	ctx         context.Context
	cancel      context.CancelFunc

	// tracker observes per-conn connectivity state and powers
	// [SubscribeTarget], [StateForTarget], and the optional health helper.
	// Always allocated; watcher goroutines are spawned only when a consumer
	// has opted in.
	tracker *stateTracker
	// health is the optional observability/health helper. Always allocated
	// (zero-overhead when no coordinator is configured) so [CheckHealth]
	// works as a public API regardless of opt-in.
	health *poolHealth
}

// targetPool manages connections for a specific target address.
type targetPool struct {
	pool    *ConnectionPool
	target  string
	opts    *options
	logger  *slog.Logger
	mu      sync.Mutex
	active  map[*grpc.ClientConn]*pooledConnection
	idle    []*pooledConnection
	changed chan struct{}
	closed  bool
	dialing int
	creates sync.WaitGroup
}

// pooledConnection wraps a gRPC client connection with pool metadata.
type pooledConnection struct {
	conn     *grpc.ClientConn
	target   string
	created  time.Time
	lastUsed atomic.Int64 // Unix nanoseconds
	inUse    atomic.Bool
	binding  *Binding
}

// New creates a new connection pool with the specified options.
// The pool is inactive until Start is called to begin background cleanup.
//
// Example:
//
//	p := pool.New(pool.WithSize(20))
//	stop, _ := p.Start(ctx)
//	defer stop()
//	conn, _ := p.GetConnection(ctx, "localhost:8080")
//	defer p.ReturnConnection(conn)
func New(opts ...Option) *ConnectionPool {
	options := newOptions(opts...)
	options.size = max(1, options.size)
	cp := &ConnectionPool{
		opts:    options,
		logger:  options.logger,
		metrics: newPoolMetrics(options.collector, options.metricsSubsystem),
		tracker: newStateTracker(),
		done:    make(chan struct{}),
	}
	// Install the callback once before concurrent tracker use.
	cp.ctx, cp.cancel = context.WithCancel(context.Background())
	cp.health = newPoolHealth(cp)
	cp.tracker.onTargetStateChange = cp.health.onTargetStateChange
	return cp
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
	cp.lifecycleMu.Lock()
	defer cp.lifecycleMu.Unlock()
	if cp.stopped.Load() {
		return nil, ErrConnectionPoolClosed
	}
	if cp.started {
		return nil, ErrAlreadyStarted
	}
	cp.started = true
	cp.health.register() //nolint:contextcheck // Watchers use the tracker-owned lifetime.
	loopDone := make(chan struct{})
	// This goroutine owns the periodic cleanup lifecycle, not finite fan-out.
	go func() {
		defer close(loopDone)
		ticker := time.NewTicker(cp.opts.cleanupInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				cp.shutdown()
				return
			case <-cp.ctx.Done():
				return
			case <-ticker.C:
				cp.cleanup() //nolint:contextcheck // Periodic cleanup is pool-owned, not a request operation.
			}
		}
	}()
	return func() { cp.shutdown(); <-loopDone }, nil
}

func (cp *ConnectionPool) shutdown() {
	cp.stopOnce.Do(func() {
		cp.lifecycleMu.Lock()
		cp.stopped.Store(true)
		cp.cancel()
		cp.lifecycleMu.Unlock()
		cp.pools.Range(func(_, value any) bool {
			tp, _ := value.(*targetPool) //nolint:errcheck // Only targetPool values are stored.
			tp.close()
			return true
		})
		cp.tracker.shutdown()
		close(cp.done)
	})
	<-cp.done
}

// GetConnection borrows a connection using the pool's default factory. At the
// per-target limit it waits for a return or for ctx cancellation. The default
// factory uses TLS; plaintext requires an explicit WithClientFactory.
func (cp *ConnectionPool) GetConnection(ctx context.Context, target string) (*grpc.ClientConn, error) {
	return cp.getConnection(ctx, target, nil)
}

func (cp *ConnectionPool) getConnection(ctx context.Context, target string, binding *Binding) (*grpc.ClientConn, error) {
	tp, err := cp.getOrCreateTargetPool(target)
	if err != nil {
		return nil, err
	}
	return tp.getConnection(ctx, binding)
}

// Binding owns a connection policy for one caller and target. Connections never
// cross bindings, but all bindings share the pool's per-target capacity limit.
// The pool owns shutdown; return borrowed connections through ReturnConnection.
type Binding struct {
	pool    *ConnectionPool
	target  string
	factory ClientFactory
}

// Bind isolates a caller's factory from all other policies for the same target.
// The factory must honor cancellation and return a fresh connection on success.
func (cp *ConnectionPool) Bind(target string, factory ClientFactory) (*Binding, error) {
	if factory == nil {
		return nil, ErrFactoryRequired
	}
	cp.lifecycleMu.Lock()
	defer cp.lifecycleMu.Unlock()
	if cp.stopped.Load() {
		return nil, ErrConnectionPoolClosed
	}
	return &Binding{pool: cp, target: target, factory: factory}, nil
}

// GetConnection borrows a connection using this binding's policy.
func (b *Binding) GetConnection(ctx context.Context) (*grpc.ClientConn, error) {
	return b.pool.getConnection(ctx, b.target, b)
}

// SubscribeTarget delivers per-conn [connectivity.State] updates for the
// given target. The first call across the pool implicitly enables the state
// tracker so existing conns gain watcher goroutines. Returns
// [ErrConnectionPoolClosed] if the pool has already been shut down.
//
// The returned function unsubscribes; calling it after Close is a no-op.
// Multiple subscribers for the same target are supported.
func (cp *ConnectionPool) SubscribeTarget(target string, cb func(connectivity.State)) (func(), error) {
	if cp.stopped.Load() {
		return nil, ErrConnectionPoolClosed
	}
	return cp.tracker.subscribe(target, cb), nil
}

// StateForTarget returns the best (most-ready) [connectivity.State] across
// all conns currently tracked for target. Returns [connectivity.Idle] when
// the target is unknown or has no live conns.
func (cp *ConnectionPool) StateForTarget(target string) connectivity.State {
	return cp.tracker.stateForTarget(target)
}

// CheckHealth implements [observability/health.Checker] for the aggregate
// pool service. It returns the worst per-target status across the pool, or
// [health.StatusServing] when no targets are tracked.
func (cp *ConnectionPool) CheckHealth(ctx context.Context) health.ServingStatus {
	return cp.health.CheckHealth(ctx)
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
func (cp *ConnectionPool) getOrCreateTargetPool(target string) (*targetPool, error) {
	cp.lifecycleMu.Lock()
	defer cp.lifecycleMu.Unlock()
	if cp.stopped.Load() {
		return nil, ErrConnectionPoolClosed
	}
	if existing, ok := cp.pools.Load(target); ok {
		tp, _ := existing.(*targetPool) //nolint:errcheck // Only targetPool values are stored.
		return tp, nil
	}
	tp := &targetPool{pool: cp, target: target, opts: cp.opts,
		logger: cp.logger.With("target", target), active: make(map[*grpc.ClientConn]*pooledConnection), changed: make(chan struct{})}
	cp.pools.Store(target, tp)
	return tp, nil
}

// cleanup performs cleanup of idle connections across all target pools.
func (cp *ConnectionPool) cleanup() {
	stop := cp.metrics.cleanupDuration.Start()
	cleanedTotal := 0

	cp.pools.Range(func(key, value any) bool {
		tPool, ok := value.(*targetPool)
		if ok {
			cleaned := tPool.cleanup()
			cleanedTotal += cleaned
		}
		return true
	})

	stop()

	if cleanedTotal > 0 {
		cp.metrics.cleanupRemoved.Add(float64(cleanedTotal))
		//nolint:contextcheck // background cleanup goroutine has no request context
		cp.logger.DebugContext(context.Background(), "cleaned up idle connections", "count", cleanedTotal)
	}
}

// notifyLocked wakes all borrowers to re-check capacity and matching idle conns.
func (tp *targetPool) notifyLocked() {
	close(tp.changed)
	tp.changed = make(chan struct{})
}

func (tp *targetPool) getConnection(ctx context.Context, binding *Binding) (*grpc.ClientConn, error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		tp.mu.Lock()
		if tp.closed || tp.pool.stopped.Load() {
			tp.mu.Unlock()
			return nil, ErrConnectionPoolClosed
		}
		for i := len(tp.idle) - 1; i >= 0; i-- {
			pc := tp.idle[i]
			if !tp.isHealthy(pc) {
				tp.idle = append(tp.idle[:i], tp.idle[i+1:]...)
				tp.closeConnectionLocked(pc, closeReasonUnhealthy)
				continue
			}
			if pc.binding != binding {
				continue
			}
			tp.idle = append(tp.idle[:i], tp.idle[i+1:]...)
			pc.inUse.Store(true)
			pc.lastUsed.Store(time.Now().UnixNano())
			tp.pool.metrics.connectionsReused.WithLabels(metrics.Labels{"target": tp.target}).Inc()
			tp.pool.metrics.connectionsIdle.Dec()
			tp.pool.metrics.connectionsInUse.Inc()
			tp.mu.Unlock()
			return pc.conn, nil
		}
		// Evict an idle connection with a different policy rather than letting it
		// occupy the last slot forever. Borrowed connections are never evicted.
		if len(tp.active)+tp.dialing >= tp.opts.size && len(tp.idle) > 0 {
			pc := tp.idle[len(tp.idle)-1]
			tp.idle = tp.idle[:len(tp.idle)-1]
			tp.closeConnectionLocked(pc, closeReasonPoolFull)
		}
		if len(tp.active)+tp.dialing < tp.opts.size {
			tp.dialing++
			tp.creates.Add(1) // close sets closed under this mutex before waiting.
			tp.mu.Unlock()
			return tp.createConnection(ctx, binding)
		}
		changed := tp.changed
		tp.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-tp.pool.ctx.Done():
			return nil, ErrConnectionPoolClosed
		case <-changed:
		}
	}
}

func (tp *targetPool) returnConnection(conn *grpc.ClientConn) {
	tp.mu.Lock()
	defer tp.mu.Unlock()
	pc, ok := tp.active[conn]
	if !ok || !pc.inUse.Load() {
		return
	}
	if tp.closed || tp.pool.stopped.Load() {
		tp.closeConnectionLocked(pc, closeReasonShutdown)
		return
	}
	if !tp.isHealthy(pc) {
		tp.closeConnectionLocked(pc, closeReasonUnhealthy)
		return
	}
	pc.inUse.Store(false)
	pc.lastUsed.Store(time.Now().UnixNano())
	tp.pool.metrics.connectionsInUse.Dec()
	tp.pool.metrics.connectionsIdle.Inc()
	tp.idle = append(tp.idle, pc)
	tp.notifyLocked()
}

func (tp *targetPool) createConnection(ctx context.Context, binding *Binding) (*grpc.ClientConn, error) {
	defer tp.creates.Done()
	ctx, cancel := corecontext.WithMaxTimeout(ctx, tp.opts.connectTimeout)
	defer cancel()
	stopCancel := context.AfterFunc(tp.pool.ctx, cancel) //nolint:contextcheck // Cancel on either caller or pool shutdown.
	defer stopCancel()
	factory := tp.opts.clientFactory
	if binding != nil {
		factory = binding.factory
	}
	stop := tp.pool.metrics.connectDuration.WithLabels(metrics.Labels{"target": tp.target}).Start()
	var conn *grpc.ClientConn
	var err error
	if factory != nil {
		conn, err = factory(ctx, tp.target)
	} else {
		conn, err = grpc.NewClient(tp.target, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12})))
	}
	stop()
	tp.mu.Lock()
	defer tp.mu.Unlock()
	tp.dialing--
	tp.notifyLocked()
	if tp.closed || tp.pool.stopped.Load() {
		err = ErrConnectionPoolClosed
	} else if ctx.Err() != nil {
		err = ctx.Err()
	}
	if err == nil && conn == nil {
		err = errors.New("client factory returned nil connection")
	}
	if err != nil {
		if conn != nil {
			_ = conn.Close()
		}
		tp.pool.metrics.connectionErrors.WithLabels(metrics.Labels{"target": tp.target}).Inc()
		return nil, coreerrs.Wrapf(err, "failed to connect to %s", tp.target)
	}
	pc := &pooledConnection{conn: conn, target: tp.target, created: time.Now(), binding: binding}
	pc.inUse.Store(true)
	pc.lastUsed.Store(time.Now().UnixNano())
	tp.active[conn] = pc
	tp.pool.connOwner.Store(conn, tp)
	tp.pool.tracker.attach(tp.target, pc) //nolint:contextcheck // Watchers outlive the borrowing request.
	tp.pool.health.onConnAttached(tp.target)
	tp.pool.metrics.connectionsCreated.WithLabels(metrics.Labels{"target": tp.target}).Inc()
	tp.pool.metrics.connectionsActive.Inc()
	tp.pool.metrics.connectionsInUse.Inc()
	return conn, nil
}

func (tp *targetPool) isHealthy(pc *pooledConnection) bool {
	if pc == nil || pc.conn == nil {
		return false
	}
	state := pc.conn.GetState()
	return state == connectivity.Ready || state == connectivity.Idle
}

// closeConnectionLocked removes accounting exactly once. The caller removes
// pc from idle first; holding mu makes publication and closure mutually exclusive.
func (tp *targetPool) closeConnectionLocked(pc *pooledConnection, reason closeReason) {
	if _, ok := tp.active[pc.conn]; !ok {
		return
	}
	delete(tp.active, pc.conn)
	tp.pool.connOwner.Delete(pc.conn)
	tp.pool.tracker.detach(tp.target, pc)
	tp.pool.health.onConnDetached(tp.target)
	m := tp.pool.metrics
	m.connectionsClosed.WithLabels(metrics.Labels{"target": tp.target, "reason": string(reason)}).Inc()
	m.connectionsActive.Dec()
	if pc.inUse.Load() {
		m.connectionsInUse.Dec()
	} else {
		m.connectionsIdle.Dec()
	}
	_ = pc.conn.Close()
	tp.notifyLocked()
}

func (tp *targetPool) cleanup() int {
	tp.mu.Lock()
	defer tp.mu.Unlock()
	if tp.closed {
		return 0
	}
	cleaned := 0
	cutoff := time.Now().Add(-tp.opts.maxIdleTime).UnixNano()
	for i := len(tp.idle) - 1; i >= 0; i-- {
		pc := tp.idle[i]
		if pc.lastUsed.Load() < cutoff || !tp.isHealthy(pc) {
			tp.idle = append(tp.idle[:i], tp.idle[i+1:]...)
			reason := closeReasonIdle
			if !tp.isHealthy(pc) {
				reason = closeReasonUnhealthy
			}
			tp.closeConnectionLocked(pc, reason)
			cleaned++
		}
	}
	return cleaned
}

func (tp *targetPool) close() {
	tp.mu.Lock()
	tp.closed = true
	tp.idle = nil
	for _, pc := range tp.active {
		tp.closeConnectionLocked(pc, closeReasonShutdown)
	}
	tp.notifyLocked()
	tp.mu.Unlock()
	tp.creates.Wait()
}
