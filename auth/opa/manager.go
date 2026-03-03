// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package opa

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/open-policy-agent/opa/v1/rego"
	"github.com/open-policy-agent/opa/v1/storage"
	"github.com/open-policy-agent/opa/v1/storage/inmem"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// Manager manages OPA policies and provides thread-safe evaluation.
// It coordinates policy loading from a PolicySource, handles hot-reload,
// and distributes policy events to subscribers.
type Manager struct {
	source       PolicySource
	query        string
	store        storage.Store
	preparedEval atomic.Pointer[rego.PreparedEvalQuery]
	revision     atomic.Pointer[string]
	moduleCount  atomic.Int32

	lastUpdate atomic.Pointer[time.Time]
	lastError  atomic.Pointer[error]

	watchManager *watchManager
	opts         *options
	logger       *slog.Logger

	mu                  sync.Mutex
	watchCtx            context.Context
	watchStop           context.CancelFunc
	watching            bool
	closed              atomic.Bool
	updateCycleRunning  atomic.Bool // Guards against concurrent RunUpdateCycle calls.
	schedulerRegistered atomic.Bool // Marks if RunUpdateCycle is managed by scheduler.
}

// NewManager creates a new OPA Manager with the given source and query.
// The source is used to fetch policies, and the query is evaluated against inputs.
func NewManager(ctx context.Context, source PolicySource, query string, opts ...Option) (*Manager, error) {
	if source == nil {
		return nil, ErrSourceRequired
	}

	if query == "" {
		return nil, ErrQueryRequired
	}

	o := newOptions(opts...)

	m := &Manager{
		source:       source,
		query:        query,
		store:        inmem.New(),
		watchManager: newWatchManager(),
		opts:         o,
		logger:       cmp.Or(o.logger, slog.New(slog.DiscardHandler)),
	}

	// Initialize revision to empty string
	emptyRevision := ""
	m.revision.Store(&emptyRevision)

	// Perform initial policy load
	if err := m.reload(ctx); err != nil {
		return nil, coreerrs.WrapOperation(err, "load initial policies")
	}

	// Register background tasks with scheduler if provided
	if err := m.registerSchedulerTasks(ctx, o); err != nil {
		_ = m.Close()
		return nil, coreerrs.WrapOperation(err, "register scheduler tasks")
	}

	if o.healthCoordinator != nil {
		o.healthCoordinator.RegisterService("opa", m)
	}

	return m, nil
}

// Evaluator returns an Evaluator for policy checks.
// The evaluator is thread-safe and automatically uses the latest loaded policies.
func (m *Manager) Evaluator() Evaluator {
	return &regoEvaluator{
		manager: m,
	}
}

// Query returns the Rego query used for evaluation.
func (m *Manager) Query() string {
	return m.query
}

// Revision returns the current policy bundle revision.
func (m *Manager) Revision() string {
	if v := m.revision.Load(); v != nil {
		return *v
	}
	return ""
}

// Source returns the policy source.
func (m *Manager) Source() PolicySource {
	return m.source
}

// ModuleCount returns the number of policy modules currently loaded.
func (m *Manager) ModuleCount() int32 {
	return m.moduleCount.Load()
}

// IsWatching returns true if the manager is currently watching for policy changes.
func (m *Manager) IsWatching() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.watching
}

// StartWatching starts watching the policy source for changes and automatically
// reloads policies when updates are detected.
func (m *Manager) StartWatching(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed.Load() {
		return ErrManagerClosed
	}

	if m.watching {
		return nil
	}

	watchCh, err := m.source.Watch(ctx)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrWatchStartFailed, err)
	}

	m.watchCtx, m.watchStop = context.WithCancel(ctx)
	m.watching = true

	go m.watchLoop(watchCh)

	m.logger.Info("started policy watching", slog.String("source", m.source.Name()))

	return nil
}

// StopWatching stops watching for policy changes.
func (m *Manager) StopWatching() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.watching {
		return
	}

	if m.watchStop != nil {
		m.watchStop()
	}
	m.watching = false

	m.logger.Info("stopped policy watching")
}

// watchLoop handles signals from the policy source and triggers reloads.
func (m *Manager) watchLoop(watchCh <-chan struct{}) {
	for {
		select {
		case <-m.watchCtx.Done():
			return
		case _, ok := <-watchCh:
			if !ok {
				return
			}
			if err := m.runUpdateCycleInternal(m.watchCtx); err != nil {
				m.logger.Error("policy reload failed", slog.Any("error", err))
			}
		}
	}
}

// runUpdateCycleInternal performs the actual update cycle.
// It is safe to call concurrently; if already running, returns immediately.
func (m *Manager) runUpdateCycleInternal(ctx context.Context) error {
	// Prevent concurrent execution
	if !m.updateCycleRunning.CompareAndSwap(false, true) {
		return nil // Already running, skip this cycle
	}
	defer m.updateCycleRunning.Store(false)

	if m.closed.Load() {
		return nil
	}

	return m.reload(ctx)
}

// reload fetches and loads policies from the source.
func (m *Manager) reload(ctx context.Context) error {
	bundle, err := m.source.Fetch(ctx)
	if err != nil {
		m.lastError.Store(&err)
		m.broadcastError(err)
		return fmt.Errorf("%w: %w", ErrBundleFetchFailed, err)
	}

	currentRevision := m.Revision()
	if bundle.Revision == currentRevision {
		m.logger.Debug("bundle unchanged, skipping reload", slog.String("revision", bundle.Revision))
		return nil
	}

	// Prepare new query with the fetched modules
	store := inmem.New()
	if len(bundle.Data) > 0 {
		if loadErr := loadBundleData(ctx, store, bundle.Data); loadErr != nil {
			m.lastError.Store(&loadErr)
			m.broadcastError(loadErr)
			return fmt.Errorf("%w: %w", ErrQueryPrepareFailed, loadErr)
		}
	}

	regoOpts := []func(*rego.Rego){
		rego.Query(m.query),
		rego.Store(store),
	}

	for path, content := range bundle.Modules {
		regoOpts = append(regoOpts, rego.Module(path, string(content)))
	}

	r := rego.New(regoOpts...)

	pq, err := r.PrepareForEval(ctx)
	if err != nil {
		m.lastError.Store(&err)
		m.broadcastError(err)
		return fmt.Errorf("%w: %w", ErrQueryPrepareFailed, err)
	}

	// Atomically update the prepared query
	m.preparedEval.Store(&pq)
	m.revision.Store(&bundle.Revision)
	m.moduleCount.Store(int32(min(bundle.ModuleCount(), 1<<31-1))) //#nosec G115 -- capped at MaxInt32
	m.store = store
	now := time.Now()
	m.lastUpdate.Store(&now)
	m.lastError.Store(nil)

	m.logger.Info("policies reloaded",
		slog.String("source", m.source.Name()),
		slog.String("revision", bundle.Revision),
		slog.String("previous_revision", currentRevision),
		slog.Int("modules", bundle.ModuleCount()))

	// Broadcast update event
	m.watchManager.broadcast(PolicyEvent{
		Type:             EventTypePolicyUpdated,
		Source:           m.source.Name(),
		Revision:         bundle.Revision,
		PreviousRevision: currentRevision,
		OccurredAt:       time.Now(),
	})

	return nil
}

func loadBundleData(ctx context.Context, store storage.Store, data map[string]any) error {
	txn, err := store.NewTransaction(ctx, storage.WriteParams)
	if err != nil {
		return err
	}
	defer store.Abort(ctx, txn)

	for key, value := range data {
		path, ok := storage.ParsePath("/" + key)
		if !ok {
			return fmt.Errorf("invalid data path %q", key)
		}
		if err := store.Write(ctx, txn, storage.AddOp, path, value); err != nil {
			return coreerrs.Wrapf(err, "write data %q", key)
		}
	}

	return store.Commit(ctx, txn)
}

// broadcastError sends an error event to all subscribers.
func (m *Manager) broadcastError(err error) {
	m.watchManager.broadcast(PolicyEvent{
		Type:       EventTypePolicyError,
		Source:     m.source.Name(),
		OccurredAt: time.Now(),
		Error:      err,
	})
}

// Watch creates a subscription to policy events.
// Events are delivered when policies are updated or when errors occur.
func (m *Manager) Watch(ctx context.Context, opts WatchOptions) (*WatchResult, error) {
	if m.closed.Load() {
		return nil, ErrManagerClosed
	}

	return m.watchManager.subscribe(opts), nil
}

// Close releases all resources and stops watching.
func (m *Manager) Close() error {
	if !m.closed.CompareAndSwap(false, true) {
		return nil
	}

	m.StopWatching()
	m.watchManager.close()

	if err := m.source.Close(); err != nil {
		return coreerrs.WrapOperation(err, "close source")
	}

	m.logger.Info("manager closed")

	return nil
}

// regoEvaluator implements the Evaluator interface.
type regoEvaluator struct {
	manager *Manager
}

// Evaluate evaluates the policy with the given input.
func (e *regoEvaluator) Evaluate(ctx context.Context, input any) (*Result, error) {
	pq := e.manager.preparedEval.Load()
	if pq == nil {
		return nil, ErrPoliciesNotLoaded
	}

	results, err := pq.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "evaluate policy")
	}

	if len(results) == 0 {
		return &Result{Allow: false}, nil
	}

	if len(results[0].Expressions) == 0 {
		return e.buildResult(false), nil
	}

	if allow, ok := results[0].Expressions[0].Value.(bool); ok {
		return e.buildResult(allow), nil
	}

	return e.buildResult(false), nil
}

// Pre-allocated singleton results for the common non-logging path,
// avoiding a heap allocation on every policy evaluation.
var (
	resultAllow = &Result{Allow: true}
	resultDeny  = &Result{Allow: false}
)

// buildResult creates a Result with optional DecisionID based on logging settings.
func (e *regoEvaluator) buildResult(allow bool) *Result {
	if !e.manager.opts.decisionLogging {
		if allow {
			return resultAllow
		}
		return resultDeny
	}

	return &Result{Allow: allow, DecisionID: uuid.NewString()}
}

// Query returns the Rego query used for evaluation.
func (e *regoEvaluator) Query() string {
	return e.manager.query
}

// Compile-time interface checks.
var _ Evaluator = (*regoEvaluator)(nil)
