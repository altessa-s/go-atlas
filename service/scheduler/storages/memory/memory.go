// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory

import (
	"cmp"
	"context"
	"iter"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/altessa-s/go-atlas/data/filter"
	"github.com/altessa-s/go-atlas/service/scheduler"

	coremaps "github.com/altessa-s/go-atlas/core/collections/maps"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// Storage is an in-memory implementation of [scheduler.Storage] that keeps task
// state and execution history in plain Go maps protected by a [sync.RWMutex].
//
// All methods are safe for concurrent use. Returned [scheduler.TaskState] and
// [scheduler.TaskHistory] values are deep copies, so callers may modify them
// without affecting the data held by Storage.
type Storage struct {
	mu               sync.RWMutex
	tasks            map[string]*scheduler.TaskState
	history          map[string][]*scheduler.TaskHistory
	maxHist          int // Maximum history entries per task
	taskEvaluator    *filter.Evaluator
	historyEvaluator *filter.Evaluator
}

// New creates a new [Storage] with empty task and history maps.
//
// maxHistoryPerTask caps how many [scheduler.TaskHistory] entries are retained
// per task. When a call to [Storage.AddHistory] would exceed this limit, the
// oldest entry is silently discarded. If maxHistoryPerTask is zero or negative
// it defaults to 1000.
//
// Returns an error only if constructing the internal task / history filter
// evaluators fails, which propagates [filter.ErrAllowlistRequired] from a
// future configuration regression. Today the option set is static and the
// call cannot fail in practice — the error is exposed so callers don't have
// to assume a non-fallible contract that the underlying [filter.NewEvaluator]
// does not promise.
//
// Example:
//
//	storage, err := memory.New(100)
//	if err != nil {
//	    return err
//	}
//	sched := scheduler.New(storage)
func New(maxHistoryPerTask int) (*Storage, error) {
	if maxHistoryPerTask <= 0 {
		maxHistoryPerTask = 1000
	}

	taskEval, err := filter.NewEvaluator(filter.WithAllowedFields(scheduler.TaskFilterFields...))
	if err != nil {
		return nil, coreerrs.Wrap(err, "build task filter evaluator")
	}
	historyEval, err := filter.NewEvaluator(filter.WithAllowedFields(scheduler.HistoryFilterFields...))
	if err != nil {
		return nil, coreerrs.Wrap(err, "build history filter evaluator")
	}

	return &Storage{
		tasks:            make(map[string]*scheduler.TaskState),
		history:          make(map[string][]*scheduler.TaskHistory),
		maxHist:          maxHistoryPerTask,
		taskEvaluator:    taskEval,
		historyEvaluator: historyEval,
	}, nil
}

// GetTask retrieves the current [scheduler.TaskState] for the given id.
// It returns (nil, nil) when the task does not exist; callers should check
// for a nil state to distinguish "not found" from an error.
func (m *Storage) GetTask(_ context.Context, id string) (*scheduler.TaskState, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	state, ok := m.tasks[id]
	if !ok {
		return nil, nil //nolint:nilnil // nil state with nil error indicates "not found"
	}

	// Return a clone to prevent external mutation
	return cloneTaskState(state), nil
}

// UpsertTask inserts or replaces the [scheduler.TaskState] identified by
// state.ID. A deep copy of state is stored, so subsequent mutations by the
// caller do not affect the stored value. The method never returns an error
// for this in-memory implementation.
func (m *Storage) UpsertTask(_ context.Context, state *scheduler.TaskState) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Store a clone
	m.tasks[state.ID] = cloneTaskState(state)

	return nil
}

// ClaimRun atomically transitions the task from active→running for the
// occurrence scheduled at expectedNextRunAt. Because all access is serialized by
// the storage mutex, the read-check-write is a single critical section, so two
// concurrent callers can never both claim the same occurrence.
func (m *Storage) ClaimRun(_ context.Context, id string, expectedNextRunAt, runStartedAt int64, runID string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.tasks[id]
	if !ok || state.Status != scheduler.TaskStatusActive {
		return false, nil
	}
	if expectedNextRunAt != 0 && state.NextRunAt != expectedNextRunAt {
		return false, nil
	}

	state.Status = scheduler.TaskStatusRunning
	state.RunStartedAt = runStartedAt
	state.LastRunID = runID
	state.UpdatedAt = runStartedAt
	return true, nil
}

// DeleteTask removes the [scheduler.TaskState] and all associated
// [scheduler.TaskHistory] entries for the given id. Deleting a non-existent
// task is a no-op and does not return an error.
func (m *Storage) DeleteTask(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.tasks, id)
	delete(m.history, id)

	return nil
}

// Tasks returns an [iter.Seq2] iterator that yields every stored
// [scheduler.TaskState] sorted lexicographically by ID. Each yielded value is
// a deep copy. The read lock is held for the entire iteration, so callers
// should consume the sequence promptly to avoid blocking writers.
func (m *Storage) Tasks(_ context.Context) iter.Seq2[*scheduler.TaskState, error] {
	return func(yield func(*scheduler.TaskState, error) bool) {
		m.mu.RLock()
		defer m.mu.RUnlock()

		// Collect and sort for consistent ordering
		states := make([]*scheduler.TaskState, 0, len(m.tasks))
		for _, state := range coremaps.Map(m.tasks, func(_ string, state *scheduler.TaskState) (string, *scheduler.TaskState) {
			return "", cloneTaskState(state)
		}) {
			states = append(states, state)
		}
		slices.SortFunc(states, func(a, b *scheduler.TaskState) int {
			return cmp.Compare(a.ID, b.ID)
		})

		for _, state := range states {
			if !yield(state, nil) {
				return
			}
		}
	}
}

// AddHistory appends a [scheduler.TaskHistory] entry for the task identified by
// history.TaskID. A shallow copy of the struct is stored. If the per-task
// history limit (set via [New]) is exceeded, the oldest entry is evicted.
func (m *Storage) AddHistory(_ context.Context, history *scheduler.TaskHistory) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Store a clone
	clone := *history
	m.history[history.TaskID] = append(m.history[history.TaskID], &clone)

	// Trim if over limit
	if len(m.history[history.TaskID]) > m.maxHist {
		m.history[history.TaskID] = m.history[history.TaskID][1:]
	}

	return nil
}

// History returns an [iter.Seq2] iterator that yields [scheduler.TaskHistory]
// entries for the given task id, ordered by StartedAt descending (most recent
// first). Each yielded value is a shallow copy. If no history exists for the
// task the iterator yields zero elements. The read lock is held for the entire
// iteration.
func (m *Storage) History(_ context.Context, id string) iter.Seq2[*scheduler.TaskHistory, error] {
	return func(yield func(*scheduler.TaskHistory, error) bool) {
		m.mu.RLock()
		defer m.mu.RUnlock()

		hist, ok := m.history[id]
		if !ok || len(hist) == 0 {
			return
		}

		// Sort descending by start time
		sorted := make([]*scheduler.TaskHistory, len(hist))
		copy(sorted, hist)
		slices.SortFunc(sorted, func(a, b *scheduler.TaskHistory) int {
			return cmp.Compare(b.StartedAt, a.StartedAt)
		})

		for _, h := range sorted {
			clone := *h
			if !yield(&clone, nil) {
				return
			}
		}
	}
}

// CleanupHistory deletes all [scheduler.TaskHistory] entries whose EndedAt
// timestamp is older than time.Now minus the given retention duration. The
// operation scans every task's history slice in-place.
func (m *Storage) CleanupHistory(_ context.Context, retention time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cutoff := time.Now().Add(-retention).Unix()

	for taskID, hist := range m.history {
		// Use slices.DeleteFunc for efficient in-place filtering
		m.history[taskID] = slices.DeleteFunc(hist, func(h *scheduler.TaskHistory) bool {
			return h.EndedAt < cutoff
		})
	}

	return nil
}

// cloneTaskState creates a deep copy of a TaskState.
func cloneTaskState(state *scheduler.TaskState) *scheduler.TaskState {
	clone := *state
	if state.Meta != nil {
		clone.Meta = coremaps.Merge(state.Meta, nil)
	}
	return &clone
}

// TasksPaginated returns up to (pg.Limit+1) task states whose ID is
// lexicographically greater than pg.AfterID, sorted by ID ascending.
// When f is non-nil, only tasks matching the filter are included.
func (m *Storage) TasksPaginated(_ context.Context, pg scheduler.Pagination, f filter.Node) ([]*scheduler.TaskState, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Collect and sort by ID
	states := make([]*scheduler.TaskState, 0, len(m.tasks))
	for _, state := range m.tasks {
		states = append(states, cloneTaskState(state))
	}
	slices.SortFunc(states, func(a, b *scheduler.TaskState) int {
		return cmp.Compare(a.ID, b.ID)
	})

	// Binary search for the start position after afterID
	startIdx := 0
	if pg.AfterID != "" {
		startIdx = sort.Search(len(states), func(i int) bool {
			return states[i].ID > pg.AfterID
		})
	}

	if f == nil {
		// No filter: return up to limit+1 items directly
		endIdx := startIdx + int(pg.Limit) + 1
		if endIdx > len(states) {
			endIdx = len(states)
		}
		return states[startIdx:endIdx], nil
	}

	// With filter: scan and collect matching items up to limit+1
	var results []*scheduler.TaskState
	for _, state := range states[startIdx:] {
		match, err := m.taskEvaluator.Evaluate(f, taskStateToFilterMap(state))
		if err != nil {
			return nil, err
		}
		if match {
			results = append(results, state)
			if int64(len(results)) > pg.Limit {
				break
			}
		}
	}
	return results, nil
}

// HistoryPaginated returns up to (pg.Limit+1) history entries for taskID,
// sorted by StartedAt descending with ID descending as a tie-breaker.
// When f is non-nil, only entries matching the filter are included.
func (m *Storage) HistoryPaginated(_ context.Context, taskID string, pg scheduler.HistoryPagination, f filter.Node) ([]*scheduler.TaskHistory, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	hist, ok := m.history[taskID]
	if !ok || len(hist) == 0 {
		return nil, nil
	}

	// Sort descending by StartedAt, then descending by ID (tie-breaker)
	sorted := make([]*scheduler.TaskHistory, len(hist))
	copy(sorted, hist)
	slices.SortFunc(sorted, func(a, b *scheduler.TaskHistory) int {
		if c := cmp.Compare(b.StartedAt, a.StartedAt); c != 0 {
			return c
		}
		return cmp.Compare(b.ID, a.ID)
	})

	// Find cursor position: the first entry strictly past the cursor in
	// descending order. An entry is past the cursor if its StartedAt is less
	// than afterStartedAt, or if StartedAt is equal but ID is less.
	startIdx := 0
	if pg.AfterID != "" {
		found := false
		for i, h := range sorted {
			if h.StartedAt < pg.AfterStartedAt || (h.StartedAt == pg.AfterStartedAt && h.ID < pg.AfterID) {
				startIdx = i
				found = true
				break
			}
		}
		if !found {
			return nil, nil
		}
	}

	if f == nil {
		// No filter: return up to limit+1 items directly
		endIdx := startIdx + int(pg.Limit) + 1
		if endIdx > len(sorted) {
			endIdx = len(sorted)
		}

		result := make([]*scheduler.TaskHistory, 0, endIdx-startIdx)
		for _, h := range sorted[startIdx:endIdx] {
			clone := *h
			result = append(result, &clone)
		}
		return result, nil
	}

	// With filter: scan and collect matching items up to limit+1
	var results []*scheduler.TaskHistory
	for _, h := range sorted[startIdx:] {
		match, err := m.historyEvaluator.Evaluate(f, taskHistoryToFilterMap(h))
		if err != nil {
			return nil, err
		}
		if match {
			clone := *h
			results = append(results, &clone)
			if int64(len(results)) > pg.Limit {
				break
			}
		}
	}
	return results, nil
}

// taskStateToFilterMap converts a TaskState to a map for filter evaluation.
// Field names use proto camelCase to match CEL expressions.
func taskStateToFilterMap(s *scheduler.TaskState) map[string]any {
	return map[string]any{
		"id":             s.ID,
		"description":    s.Description,
		"status":         int64(s.Status),
		"priority":       int64(s.Priority),
		"schedule":       s.Schedule,
		"lastRunAt":      s.LastRunAt,
		"nextRunAt":      s.NextRunAt,
		"skipNextRun":    s.SkipNextRun,
		"disableHistory": s.DisableHistory,
		"unmanaged":      s.Unmanaged,
		"oneShot":        s.OneShot,
		"failures":       int64(s.Failures),
	}
}

// taskHistoryToFilterMap converts a TaskHistory to a map for filter evaluation.
func taskHistoryToFilterMap(h *scheduler.TaskHistory) map[string]any {
	return map[string]any{
		"id":         h.ID,
		"taskId":     h.TaskID,
		"runId":      h.RunID,
		"startedAt":  h.StartedAt,
		"endedAt":    h.EndedAt,
		"durationMs": h.DurationMs,
		"success":    h.Success,
		"error":      h.Error,
	}
}

// Compile-time interface check
var _ scheduler.Storage = (*Storage)(nil)
