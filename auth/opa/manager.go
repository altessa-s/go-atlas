// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package opa

import (
	"cmp"
	"context"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"uuid"

	"github.com/open-policy-agent/opa/v1/ast"
	"github.com/open-policy-agent/opa/v1/rego"
	"github.com/open-policy-agent/opa/v1/storage"
	"github.com/open-policy-agent/opa/v1/storage/inmem"

	"github.com/altessa-s/go-atlas-core/runtime/cycle"
	"github.com/altessa-s/go-atlas-core/types/nilcheck"
	"github.com/altessa-s/go-atlas/audit/decision"
	"github.com/altessa-s/go-atlas/observability/metrics"

	coremaps "github.com/altessa-s/go-atlas-core/collections/maps"
	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

// Manager manages OPA policies and provides thread-safe evaluation.
// It coordinates policy loading from a PolicySource, handles hot-reload,
// and distributes policy events to subscribers.
type Manager struct {
	source      PolicySource
	query       string
	snapshot    atomic.Pointer[policySnapshot]
	moduleCount atomic.Int32

	lastUpdate atomic.Pointer[time.Time]
	lastError  atomic.Pointer[error]

	unregisterHealth func()
	watchManager     *watchManager
	opts             *options
	metrics          *opaMetrics
	logger           *slog.Logger

	mu        sync.Mutex
	watchStop context.CancelFunc
	watching  bool
	// watchDone is the completion channel of the most recent poll loop. It is
	// kept after the loop exits (closed) so StopWatching and Close can always
	// join it, and a new loop is chained behind it.
	watchDone chan struct{}
	closed    atomic.Bool
	// closeDone is closed once the first Close has finished its teardown;
	// concurrent Close callers wait on it.
	closeDone       chan struct{}
	shutdownCtx     context.Context
	shutdownCancel  context.CancelFunc
	reloadWG        sync.WaitGroup
	scheduled       *schedulerCallback
	updateCycleTask *cycle.Guard // Guards RunUpdateCycle and marks scheduler management.
}

// NewManager creates a new OPA Manager with the given source and query.
// On success the manager owns the source. On failure the caller must close it.
func NewManager(ctx context.Context, source PolicySource, query string, opts ...Option) (*Manager, error) {
	if source == nil {
		return nil, ErrSourceRequired
	}

	if query == "" {
		return nil, ErrQueryRequired
	}

	o := newOptions(opts...)
	if o.updateSchedule != "" && nilcheck.IsNil(o.scheduler) {
		return nil, ErrSchedulerRequired
	}

	m := &Manager{
		source:       source,
		query:        query,
		watchManager: newWatchManager(),
		opts:         o,
		metrics:      newOpaMetrics(o.meter),
		logger:       cmp.Or(o.logger, slog.New(slog.DiscardHandler)),
		closeDone:    make(chan struct{}),
	}
	m.shutdownCtx, m.shutdownCancel = context.WithCancel(context.Background())
	m.scheduled = &schedulerCallback{}
	m.updateCycleTask = cycle.New(m.runUpdateCycleInternal)
	m.scheduled.guard.Store(m.updateCycleTask)

	// Perform initial policy load
	if err := m.reload(ctx); err != nil {
		m.shutdownCancel()
		return nil, coreerrs.WrapOperation(err, "load initial policies")
	}

	// Register background tasks with scheduler if provided
	if err := m.registerSchedulerTasks(ctx, o); err != nil {
		_ = m.shutdown(false)
		return nil, coreerrs.WrapOperation(err, "register scheduler tasks")
	}

	if o.healthCoordinator != nil {
		m.registerHealth(o.healthCoordinator)
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
	if snap := m.snapshot.Load(); snap != nil {
		return snap.revision
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

	watchCtx, watchStop := context.WithCancel(ctx)
	prevDone := m.watchDone
	done := make(chan struct{})
	m.watchStop = watchStop
	m.watchDone = done
	m.watching = true

	// The loop owns its context: a later StartWatching never changes what an
	// earlier loop observes. It starts only after the previous loop exited.
	go func() {
		defer func() {
			m.mu.Lock()
			if m.watchDone == done {
				m.watching = false
				m.watchStop = nil
			}
			close(done)
			m.mu.Unlock()
		}()
		if prevDone != nil {
			<-prevDone
		}
		m.pollLoop(watchCtx)
	}()

	m.logger.Info("started policy watching",
		slog.String("source", m.source.Name()),
		slog.Duration("interval", m.opts.pollInterval))

	return nil
}

// StartWatchingUntilClose starts polling for the lifetime of the manager.
// StopWatching can end the poll loop earlier.
func (m *Manager) StartWatchingUntilClose() error { return m.StartWatching(m.shutdownCtx) }

// StopWatching stops watching for policy changes and waits for the poll loop
// to exit.
func (m *Manager) StopWatching() {
	m.mu.Lock()
	if m.watching {
		m.watchStop()
		m.watching = false
		m.logger.Info("stopped policy watching")
	}
	done := m.watchDone
	m.mu.Unlock()

	// Join outside the lock so a concurrent StartWatching is not blocked; its
	// loop is chained behind this one.
	if done != nil {
		<-done
	}
}

// pollLoop periodically fetches from the source and reloads if changed,
// until ctx is canceled.
func (m *Manager) pollLoop(ctx context.Context) {
	ticker := time.NewTicker(m.opts.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := m.updateCycleTask.TryRun(ctx); err != nil {
				m.logger.Error("policy reload failed", slog.Any("error", err))
			}
		}
	}
}

// runUpdateCycleInternal performs the actual update cycle.
// Callers must route through updateCycleTask so overlapping cycles collapse
// into a single execution.
func (m *Manager) runUpdateCycleInternal(ctx context.Context) error {
	m.mu.Lock()
	if m.closed.Load() {
		m.mu.Unlock()
		return nil
	}
	m.reloadWG.Add(1)
	reloadCtx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(m.shutdownCtx, cancel) //nolint:contextcheck // Calls also inherit the independent manager lifetime.
	m.mu.Unlock()

	defer m.reloadWG.Done()
	defer cancel()
	defer stop()
	return m.reload(reloadCtx)
}

// reload fetches and loads policies from the source.
func (m *Manager) reload(ctx context.Context) error {
	stop := m.metrics.reloadDuration.Start()
	defer stop()

	bundle, err := m.source.Fetch(ctx)
	if err != nil {
		m.lastError.Store(&err)
		m.broadcastError(err)
		m.metrics.policyReloads.WithLabels(metrics.Labels{labelResult: "fetch_error"}).Inc()
		return coreerrs.JoinWrap(ErrBundleFetchFailed, err)
	}

	currentRevision := m.Revision()
	if bundle.Revision == currentRevision && m.snapshot.Load() != nil {
		m.lastError.Store(nil)
		m.logger.Debug("bundle unchanged, skipping reload", slog.String("revision", bundle.Revision))
		m.metrics.policyReloads.WithLabels(metrics.Labels{labelResult: "unchanged"}).Inc()
		return nil
	}

	// Prepare new query with the fetched modules
	store := inmem.New()
	if len(bundle.Data) > 0 {
		if loadErr := loadBundleData(ctx, store, bundle.Data); loadErr != nil {
			m.lastError.Store(&loadErr)
			m.broadcastError(loadErr)
			m.metrics.policyReloads.WithLabels(metrics.Labels{labelResult: "prepare_error"}).Inc()
			return coreerrs.JoinWrap(ErrQueryPrepareFailed, loadErr)
		}
	}

	regoOpts := []func(*rego.Rego){
		rego.Query(m.query),
		rego.Store(store),
	}

	for path, content := range bundle.Modules {
		regoOpts = append(regoOpts, rego.Module(path, string(content)))
	}

	var compiler *ast.Compiler
	if m.opts.decisionCache != nil {
		regoOpts = append(regoOpts, rego.CompilerHook(func(c *ast.Compiler) { compiler = c }))
	}
	r := rego.New(regoOpts...)

	pq, err := r.PrepareForEval(ctx)
	if err != nil {
		m.lastError.Store(&err)
		m.broadcastError(err)
		m.metrics.policyReloads.WithLabels(metrics.Labels{labelResult: "prepare_error"}).Inc()
		return coreerrs.JoinWrap(ErrQueryPrepareFailed, err)
	}

	// Atomically update the prepared query
	m.snapshot.Store(&policySnapshot{
		preparedEval:  pq,
		revision:      bundle.Revision,
		cacheEligible: compiler != nil && cacheEligible(compiler, m.query),
	})

	// Decisions are keyed by revision, so the old ones are already unreachable;
	// purging only stops them from occupying the cache until they age out.
	if local, ok := m.opts.decisionCache.(interface{ purge() }); ok {
		local.purge()
	}

	m.moduleCount.Store(int32(min(bundle.ModuleCount(), 1<<31-1))) //#nosec G115 -- capped at MaxInt32
	now := time.Now()
	m.lastUpdate.Store(&now)
	m.lastError.Store(nil)
	m.metrics.policyReloads.WithLabels(metrics.Labels{labelResult: "success"}).Inc()
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

// loadBundleData writes data in one write transaction. storage.Txn aborts
// only on failure: aborting after Commit makes the inmem store panic with
// "stale transaction".
func loadBundleData(ctx context.Context, store storage.Store, data map[string]any) error {
	return storage.Txn(ctx, store, storage.WriteParams, func(txn storage.Transaction) error {
		return writeBundleData(ctx, store, txn, data)
	})
}

func writeBundleData(ctx context.Context, store storage.Store, txn storage.Transaction, data map[string]any) error {
	for _, key := range slices.Sorted(maps.Keys(data)) {
		path := storage.Path(strings.Split(key, "/"))
		if slices.Contains(path, "") {
			return coreerrs.Wrapf(ErrInvalidDataPath, "%q", key)
		}
		for i := 1; i < len(path); i++ {
			if err := storage.MakeDir(ctx, store, txn, path[:i]); err != nil {
				return coreerrs.JoinWrap(ErrInvalidDataPath, err)
			}
		}
		if _, err := store.Read(ctx, txn, path); err == nil {
			return coreerrs.Wrapf(ErrInvalidDataPath, "duplicate document %q", key)
		} else if !storage.IsNotFound(err) {
			return err
		}
		value := data[key]
		if err := store.Write(ctx, txn, storage.AddOp, path, value); err != nil {
			return coreerrs.Wrapf(err, "write data %q", key)
		}
	}
	return nil
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
// When opts.BufferSize is zero, the manager-level WithWatchChannelSize
// value is used as the event channel buffer size.
func (m *Manager) Watch(ctx context.Context, opts WatchOptions) (*WatchResult, error) {
	if m.closed.Load() {
		return nil, ErrManagerClosed
	}

	opts.BufferSize = cmp.Or(opts.BufferSize, m.opts.watchChannelSize)

	return m.watchManager.subscribe(opts), nil
}

// Close releases all resources and stops watching. It returns once the poll
// loop, reloads, and event deliveries have finished and the source is closed.
// Concurrent callers wait for the first Close to finish.
func (m *Manager) Close() error { return m.shutdown(true) }

func (m *Manager) shutdown(ownsSource bool) error {
	m.mu.Lock()
	if m.closed.Swap(true) {
		m.mu.Unlock()
		<-m.closeDone
		return nil
	}
	m.scheduled.guard.Store(nil)
	m.shutdownCancel()
	if m.watchStop != nil {
		m.watchStop()
		m.watching = false
	}
	done := m.watchDone
	m.mu.Unlock()
	defer close(m.closeDone)

	m.watchManager.close(func() {
		if m.unregisterHealth != nil {
			m.unregisterHealth()
		}
		if done != nil {
			<-done
		}
		m.reloadWG.Wait()
	})
	if ownsSource {
		if err := m.source.Close(); err != nil {
			return coreerrs.WrapOperation(err, "close source")
		}
	}

	m.logger.Info("manager closed")

	return nil
}

// regoEvaluator implements the Evaluator interface.
type regoEvaluator struct {
	manager *Manager
}

// Evaluate evaluates the policy with the given input.
//
// When a decision cache is configured (see [WithDecisionCache]), a repeated
// input under the same policy revision skips the Rego evaluation. Everything
// downstream of the decision still happens: the result is counted in the
// evaluation metrics and passed to the audit recorder, because a decision that
// is served but never recorded is a hole in the audit trail.
func (e *regoEvaluator) Evaluate(ctx context.Context, input any) (*Result, error) {
	stop := e.manager.metrics.evaluationDuration.Start()
	defer stop()

	snap := e.manager.snapshot.Load()
	if snap == nil {
		e.manager.metrics.evalError.Inc()
		return nil, ErrPoliciesNotLoaded
	}
	value, err := parseInput(input)
	if err != nil {
		e.manager.metrics.evalError.Inc()
		return nil, coreerrs.WrapOperation(err, "parse policy input")
	}
	var key string
	if snap.cacheEligible {
		key = decisionCacheKey(e.manager.query, snap.revision, value)
		if key != "" {
			if r, ok := e.cached(key); ok {
				return e.finishEvaluation(ctx, snap, r)
			}
		}
	}
	results, err := snap.preparedEval.Eval(ctx, rego.EvalParsedInput(value))
	if err != nil {
		e.manager.metrics.evalError.Inc()
		return nil, coreerrs.WrapOperation(err, "evaluate policy")
	}

	r := e.resolveResult(results)
	e.store(snap, key, r)

	return e.finishEvaluation(ctx, snap, r)
}

// finishEvaluation counts the decision and hands it to the audit recorder. It
// runs for cache hits and misses alike, so caching cannot silently thin out the
// metrics or the audit trail.
func (e *regoEvaluator) finishEvaluation(ctx context.Context, snap *policySnapshot, r *Result) (*Result, error) {
	if r.Allow {
		e.manager.metrics.evalAllow.Inc()
	} else {
		e.manager.metrics.evalDeny.Inc()
	}

	if err := e.recordDecision(ctx, snap.revision, r); err != nil {
		return nil, err
	}

	return r, nil
}

// cached looks the decision up by key, returning a copy with its own
// DecisionID.
func (e *regoEvaluator) cached(key string) (*Result, bool) {
	r, ok := e.manager.opts.decisionCache.Get(key)
	if !ok {
		e.manager.metrics.cacheMisses.Inc()
		return nil, false
	}

	// A DecisionID identifies one authorization decision; replaying a cached
	// one under its original ID would make distinct decisions indistinguishable
	// in the audit log.
	if e.manager.opts.decisionLogging {
		r.DecisionID = uuid.New().String()
	}

	e.manager.metrics.cacheHits.Inc()

	return &r, true
}

// store caches the decision, unless a reload landed while it was being computed
// — in that case the result belongs to a bundle that is no longer current and
// caching it under either revision would be wrong.
func (e *regoEvaluator) store(snap *policySnapshot, key string, r *Result) {
	cache := e.manager.opts.decisionCache
	if cache == nil || key == "" || e.manager.snapshot.Load() != snap {
		return
	}

	cache.Put(key, *r)
}

// resolveResult maps an OPA result set onto a [Result], handling the boolean,
// structured-map, and empty/unrecognized cases (the latter two deny).
func (e *regoEvaluator) resolveResult(results rego.ResultSet) *Result {
	if len(results) == 0 || len(results[0].Expressions) == 0 {
		return e.buildResult(false)
	}

	val := results[0].Expressions[0].Value

	// Path A: Boolean result.
	// Queries like "data.authz.allow" return a plain bool.
	if allow, ok := val.(bool); ok {
		return e.buildResult(allow)
	}

	// Path B: Map result (structured).
	// Queries like "data.authz.result" return {"allow": bool, "denials": [...]}.
	if m, ok := val.(map[string]any); ok {
		return e.buildResultFromMap(m)
	}

	// Fallback: unrecognized result type -> deny.
	return e.buildResult(false)
}

// recordDecision forwards the evaluation outcome to the configured audit
// recorder, if any; without one the decision is not even built. The recorder
// drops grants under its deny-only policy. A record failure aborts an
// allowed evaluation (so nothing proceeds unrecorded) only when the recorder
// runs in required-failure mode; on a denial the error is ignored.
func (e *regoEvaluator) recordDecision(ctx context.Context, revision string, r *Result) error {
	if e.manager.opts.auditRecorder == nil {
		return nil
	}

	d := decision.Decision{
		Time:       time.Now().UTC(),
		Allowed:    r.Allow,
		Action:     e.manager.query,
		Attributes: map[string]string{"engine": "opa"},
	}
	if !r.Allow {
		d.Reason = cmp.Or(r.DecisionID, "deny")
	}
	if revision != "" {
		d.Attributes["revision"] = revision
	}

	recErr := e.manager.opts.auditRecorder.Record(ctx, d)
	if r.Allow && recErr != nil {
		return coreerrs.WrapOperation(recErr, "record decision")
	}

	return nil
}

// buildResult creates a Result with optional DecisionID based on logging settings.
func (e *regoEvaluator) buildResult(allow bool) *Result {
	if !e.manager.opts.decisionLogging {
		// Allocate a fresh Result rather than returning a shared singleton:
		// Result has exported mutable fields, so a caller enriching the value
		// would otherwise corrupt every concurrent evaluation.
		return &Result{Allow: allow}
	}

	return &Result{Allow: allow, DecisionID: uuid.New().String()}
}

// buildResultFromMap parses a structured OPA result object into a Result.
// Expected shape: {"allow": bool, "denials": [{"code": "...", "message": "..."}, ...]}.
func (e *regoEvaluator) buildResultFromMap(m map[string]any) *Result {
	r := &Result{}

	if allow, ok := m["allow"].(bool); ok {
		r.Allow = allow
	}

	if e.manager.opts.decisionLogging {
		r.DecisionID = uuid.New().String()
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
