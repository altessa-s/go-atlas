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

	"github.com/altessa-s/go-atlas/observability/metrics"

	coremaps "github.com/altessa-s/go-atlas/core/collections/maps"
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
	metrics      *opaMetrics
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
		metrics:      newOpaMetrics(o.collector),
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
// reloads policies when updates are detected. The Manager polls the source at
// the configured pollInterval.
func (m *Manager) StartWatching(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed.Load() {
		return ErrManagerClosed
	}

	if m.watching {
		return nil
	}

	m.watchCtx, m.watchStop = context.WithCancel(ctx)
	m.watching = true

	go m.pollLoop()

	m.logger.Info("started policy watching",
		slog.String("source", m.source.Name()),
		slog.Duration("interval", m.opts.pollInterval))

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

// pollLoop periodically fetches from the source and reloads if changed.
func (m *Manager) pollLoop() {
	ticker := time.NewTicker(m.opts.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.watchCtx.Done():
			return
		case <-ticker.C:
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
	stop := m.metrics.reloadDuration.Start()
	defer stop()

	bundle, err := m.source.Fetch(ctx)
	if err != nil {
		m.lastError.Store(&err)
		m.broadcastError(err)
		m.metrics.policyReloads.WithLabels(metrics.Labels{"result": "fetch_error"}).Inc()
		return fmt.Errorf("%w: %w", ErrBundleFetchFailed, err)
	}

	currentRevision := m.Revision()
	if bundle.Revision == currentRevision {
		m.logger.Debug("bundle unchanged, skipping reload", slog.String("revision", bundle.Revision))
		m.metrics.policyReloads.WithLabels(metrics.Labels{"result": "unchanged"}).Inc()
		return nil
	}

	// Prepare new query with the fetched modules
	store := inmem.New()
	if len(bundle.Data) > 0 {
		if loadErr := loadBundleData(ctx, store, bundle.Data); loadErr != nil {
			m.lastError.Store(&loadErr)
			m.broadcastError(loadErr)
			m.metrics.policyReloads.WithLabels(metrics.Labels{"result": "prepare_error"}).Inc()
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
		m.metrics.policyReloads.WithLabels(metrics.Labels{"result": "prepare_error"}).Inc()
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
	m.metrics.policyReloads.WithLabels(metrics.Labels{"result": "success"}).Inc()
	m.metrics.modulesLoaded.Set(float64(bundle.ModuleCount()))

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
	stop := e.manager.metrics.evaluationDuration.Start()
	defer stop()

	pq := e.manager.preparedEval.Load()
	if pq == nil {
		e.manager.metrics.evaluations.WithLabels(metrics.Labels{"result": "error"}).Inc()
		return nil, ErrPoliciesNotLoaded
	}

	results, err := pq.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		e.manager.metrics.evaluations.WithLabels(metrics.Labels{"result": "error"}).Inc()
		return nil, coreerrs.WrapOperation(err, "evaluate policy")
	}

	if len(results) == 0 || len(results[0].Expressions) == 0 {
		e.manager.metrics.evaluations.WithLabels(metrics.Labels{"result": "deny"}).Inc()
		return e.buildResult(false), nil
	}

	val := results[0].Expressions[0].Value

	// Path A: Boolean result (backward compatible).
	// Queries like "data.authz.allow" return a plain bool.
	if allow, ok := val.(bool); ok {
		e.manager.metrics.evaluations.WithLabels(metrics.Labels{"result": allowDenyLabel(allow)}).Inc()
		return e.buildResult(allow), nil
	}

	// Path B: Map result (structured).
	// Queries like "data.authz.result" return {"allow": bool, "denials": [...]}.
	if m, ok := val.(map[string]any); ok {
		r := e.buildResultFromMap(m)
		e.manager.metrics.evaluations.WithLabels(metrics.Labels{"result": allowDenyLabel(r.Allow)}).Inc()
		return r, nil
	}

	// Fallback: unrecognized result type -> deny.
	e.manager.metrics.evaluations.WithLabels(metrics.Labels{"result": "deny"}).Inc()
	return e.buildResult(false), nil
}

func allowDenyLabel(allow bool) string {
	if allow {
		return "allow"
	}
	return "deny"
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

// buildResultFromMap parses a structured OPA result object into a Result.
// Expected shape: {"allow": bool, "denials": [{"code": "...", "message": "..."}, ...]}.
func (e *regoEvaluator) buildResultFromMap(m map[string]any) *Result {
	r := &Result{}

	if allow, ok := m["allow"].(bool); ok {
		r.Allow = allow
	}

	if e.manager.opts.decisionLogging {
		r.DecisionID = uuid.NewString()
	}

	if raw, ok := m["denials"]; ok {
		r.Denials = parseDenials(raw)
	}

	return r
}

// parseDenials converts an OPA set/array value into an ImmutableMap of code → message.
// OPA represents sets as []any in the Go evaluation API.
func parseDenials(v any) *coremaps.ImmutableMap[string, string] {
	items, ok := v.([]any)
	if !ok {
		return nil
	}

	denials := make(map[string]string, len(items))
	for _, item := range items {
		dm, ok := item.(map[string]any)
		if !ok {
			continue
		}
		code, _ := dm["code"].(string)
		if code == "" {
			continue
		}
		msg, _ := dm["message"].(string)
		denials[code] = msg
	}

	if len(denials) == 0 {
		return nil
	}

	return coremaps.NewImmutableMap(denials)
}

// Query returns the Rego query used for evaluation.
func (e *regoEvaluator) Query() string {
	return e.manager.query
}

// Compile-time interface checks.
var _ Evaluator = (*regoEvaluator)(nil)
