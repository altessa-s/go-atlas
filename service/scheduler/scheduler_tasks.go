// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scheduler

import (
	"context"
	"errors"
	"iter"
	"log/slog"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/altessa-s/go-atlas/data/filter"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// Register adds or updates a task in the scheduler. The task is persisted to
// [Storage] and registered in the in-memory dispatch map. If a task with the
// same ID already exists, its configuration is merged: the schedule, priority,
// and description are updated, while execution history fields (LastRunAt,
// Failures, etc.) are preserved.
//
// cfg must have a non-empty ID and a non-nil Func. Exactly one of Schedule or
// RunAt must be set; providing both returns [ErrScheduleConflict]. Schedule
// strings are validated against the cron parser on registration. If Priority is
// unset, it defaults to [TaskPriorityNormal].
//
// Re-registering a [TaskStatusCompleted] one-shot task resets it to
// [TaskStatusActive], allowing it to run again.
func (s *Scheduler) Register(ctx context.Context, cfg corescheduler.TaskConfig) error {
	if cfg.ID == "" {
		return errors.New("task ID cannot be empty")
	}

	if cfg.Func == nil {
		return errors.New("task function cannot be nil")
	}

	if cfg.Priority == TaskPriorityUnspecified {
		cfg.Priority = TaskPriorityNormal
	}

	isOneShot := !cfg.RunAt.IsZero()

	if isOneShot && cfg.Schedule != "" {
		return ErrScheduleConflict
	}

	if !isOneShot && cfg.Schedule == "" {
		return errors.New("task schedule is required (e.g., '@every 5m', '0 */5 * * * *')")
	}

	// Validate schedule for periodic tasks
	var sched cron.Schedule
	if !isOneShot {
		var err error
		sched, err = s.parser.Parse(cfg.Schedule)
		if err != nil {
			return coreerrs.Wrapf(err, "invalid cron schedule %q", cfg.Schedule)
		}
		// Cache the parsed schedule so calculateNextRun avoids re-parsing.
		s.scheduleCache.Store(cfg.Schedule, sched)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	var nextRun time.Time

	if isOneShot {
		nextRun = cfg.RunAt
		if nextRun.Before(now) {
			nextRun = now
		}
	} else {
		nextRun = sched.Next(now)
		if cfg.RunOnStart {
			nextRun = now
		}
	}

	nowUnix := now.Unix()

	// Create or update storage state
	state := &TaskState{
		TaskSummary: TaskSummary{
			ID:             cfg.ID,
			Description:    cfg.Description,
			Status:         TaskStatusActive,
			Priority:       cfg.Priority,
			Schedule:       cfg.Schedule,
			NextRunAt:      nextRun.Unix(),
			DisableHistory: cfg.DisableHistory,
			Unmanaged:      cfg.Unmanaged,
			OneShot:        isOneShot,
			Failures:       0,
		},
		Meta:      cfg.Meta,
		CreatedAt: nowUnix,
		UpdatedAt: nowUnix,
	}

	// Check if task already exists
	existing, err := s.storage.GetTask(ctx, cfg.ID)
	if err != nil {
		return coreerrs.WrapOperation(err, "check existing task")
	}

	if existing != nil {
		// Preserve certain fields from existing state
		state.CreatedAt = existing.CreatedAt
		state.LastRunAt = existing.LastRunAt
		state.LastRunID = existing.LastRunID
		state.Failures = existing.Failures
		if isOneShot {
			// One-shot: always use computed nextRun
			state.NextRunAt = nextRun.Unix()
		} else {
			// Update next run time if schedule changed
			if existing.Schedule != cfg.Schedule {
				state.NextRunAt = nextRun.Unix()
			} else {
				state.NextRunAt = existing.NextRunAt
			}
		}
		// Re-registering a completed task resets it to active
		if existing.Status == TaskStatusCompleted {
			state.Status = TaskStatusActive
		} else if existing.Status != TaskStatusActive {
			// Preserve status if not active
			state.Status = existing.Status
		}
		// Description is always updated from config (allows changing description without recreating task)
	}

	if err := s.storage.UpsertTask(ctx, state); err != nil {
		return coreerrs.WrapOperation(err, "save task state")
	}

	s.tasks[cfg.ID] = &registeredTask{
		config: cfg,
	}

	logAttrs := []any{
		slog.String("task_id", cfg.ID),
		slog.String("priority", cfg.Priority.String()),
	}
	if isOneShot {
		logAttrs = append(logAttrs, slog.Bool("one_shot", true), slog.Time("run_at", cfg.RunAt))
	} else {
		logAttrs = append(logAttrs, slog.String("schedule", cfg.Schedule), slog.Bool("run_on_start", cfg.RunOnStart))
	}
	s.logger.InfoContext(ctx, "task registered", logAttrs...)

	return nil
}

// Unregister removes a task from both the in-memory dispatch map and the
// [Storage] backend. Any currently running instance of the task will complete
// normally; only future dispatches are prevented.
//
// Returns [ErrTaskNotRegistered] if the task ID is not present in the
// in-memory map.
func (s *Scheduler) Unregister(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.tasks[id]; !ok {
		return ErrTaskNotRegistered
	}

	delete(s.tasks, id)

	if err := s.storage.DeleteTask(ctx, id); err != nil {
		return coreerrs.WrapOperation(err, "delete task from storage")
	}

	s.logger.InfoContext(ctx, "task unregistered", slog.String("task_id", id))

	return nil
}

// PauseTask transitions a task to [TaskStatusPaused], preventing future
// executions until [Scheduler.ResumeTask] is called. A currently running
// execution will complete normally.
//
// Returns [ErrTaskNotFound] if no task with the given ID exists in storage,
// [ErrTaskCompleted] if the task is a completed one-shot, or
// [ErrTaskUnmanaged] if the task has its Unmanaged flag set.
func (s *Scheduler) PauseTask(ctx context.Context, id string) error {
	state, err := s.storage.GetTask(ctx, id)
	if err != nil {
		return err
	}

	if state == nil {
		return ErrTaskNotFound
	}

	if state.Status == TaskStatusCompleted {
		return ErrTaskCompleted
	}

	if state.Unmanaged {
		return ErrTaskUnmanaged
	}

	state.Status = TaskStatusPaused
	state.UpdatedAt = UnixNow()

	return s.storage.UpsertTask(ctx, state)
}

// ResumeTask transitions a [TaskStatusPaused] task back to [TaskStatusActive]
// and computes the next run time from the current moment. For one-shot tasks
// whose scheduled time has already passed, the next run is set to now.
//
// Returns [ErrTaskNotFound] if no task with the given ID exists in storage,
// [ErrTaskCompleted] if the task is a completed one-shot, or [ErrTaskNotPaused]
// if the task's current status is not [TaskStatusPaused].
func (s *Scheduler) ResumeTask(ctx context.Context, id string) error {
	state, err := s.storage.GetTask(ctx, id)
	if err != nil {
		return err
	}

	if state == nil {
		return ErrTaskNotFound
	}

	if state.Status == TaskStatusCompleted {
		return ErrTaskCompleted
	}

	if state.Status != TaskStatusPaused {
		return ErrTaskNotPaused
	}

	now := time.Now()
	state.Status = TaskStatusActive
	if state.OneShot {
		// One-shot: keep existing NextRunAt or set to now if already past
		if state.NextRunAt < now.Unix() {
			state.NextRunAt = now.Unix()
		}
	} else {
		state.NextRunAt = s.calculateNextRun(ctx, now, state).Unix()
	}
	state.UpdatedAt = now.Unix()

	return s.storage.UpsertTask(ctx, state)
}

// DisableTask transitions a task to [TaskStatusDisabled], preventing both
// scheduled and manual execution (via [Scheduler.TriggerTask]). Use
// [Scheduler.EnableTask] to re-activate. Unlike [Scheduler.PauseTask],
// disabling is allowed regardless of the current status.
//
// Returns [ErrTaskNotFound] if no task with the given ID exists in storage,
// or [ErrTaskUnmanaged] if the task has its Unmanaged flag set.
func (s *Scheduler) DisableTask(ctx context.Context, id string) error {
	state, err := s.storage.GetTask(ctx, id)
	if err != nil {
		return err
	}

	if state == nil {
		return ErrTaskNotFound
	}

	if state.Unmanaged {
		return ErrTaskUnmanaged
	}

	state.Status = TaskStatusDisabled
	state.UpdatedAt = UnixNow()

	return s.storage.UpsertTask(ctx, state)
}

// EnableTask transitions a [TaskStatusDisabled] task back to [TaskStatusActive]
// and recomputes the next run time from the current moment. For one-shot tasks
// whose scheduled time has already passed, the next run is set to now.
//
// Returns [ErrTaskNotFound] if no task with the given ID exists in storage,
// [ErrTaskCompleted] if the task is a completed one-shot, or
// [ErrTaskNotDisabled] if the task's current status is not [TaskStatusDisabled].
func (s *Scheduler) EnableTask(ctx context.Context, id string) error {
	state, err := s.storage.GetTask(ctx, id)
	if err != nil {
		return err
	}

	if state == nil {
		return ErrTaskNotFound
	}

	if state.Status == TaskStatusCompleted {
		return ErrTaskCompleted
	}

	if state.Status != TaskStatusDisabled {
		return ErrTaskNotDisabled
	}

	now := time.Now()
	state.Status = TaskStatusActive
	if state.OneShot {
		// One-shot: keep existing NextRunAt or set to now if already past
		if state.NextRunAt < now.Unix() {
			state.NextRunAt = now.Unix()
		}
	} else {
		state.NextRunAt = s.calculateNextRun(ctx, now, state).Unix()
	}
	state.UpdatedAt = now.Unix()

	return s.storage.UpsertTask(ctx, state)
}

// SkipNextRun marks the next scheduled execution of a task to be skipped. When
// the scheduler's tick loop encounters a due task with the skip flag set, it
// clears the flag and advances NextRunAt to the following scheduled time without
// invoking the task function. For one-shot tasks, skipping transitions the task
// to [TaskStatusCompleted].
//
// Returns [ErrTaskNotFound] if no task with the given ID exists in storage.
func (s *Scheduler) SkipNextRun(ctx context.Context, id string) error {
	state, err := s.storage.GetTask(ctx, id)
	if err != nil {
		return err
	}

	if state == nil {
		return ErrTaskNotFound
	}

	state.SkipNextRun = true
	state.UpdatedAt = UnixNow()

	return s.storage.UpsertTask(ctx, state)
}

// GetTaskState retrieves the current persistent state of a task from [Storage].
// Returns (nil, nil) if no task with the given ID exists.
func (s *Scheduler) GetTaskState(ctx context.Context, id string) (*TaskState, error) {
	return s.storage.GetTask(ctx, id)
}

// Tasks returns an iterator over [TaskSummary] values for all registered tasks.
// The iteration order depends on the underlying [Storage] implementation. Use
// slices.Collect to materialize the results as a slice when random access is
// needed.
func (s *Scheduler) Tasks(ctx context.Context) iter.Seq2[*TaskSummary, error] {
	return func(yield func(*TaskSummary, error) bool) {
		for state, err := range s.storage.Tasks(ctx) {
			if err != nil {
				yield(nil, err)
				return
			}
			summary := state.TaskSummary
			if !yield(&summary, nil) {
				return
			}
		}
	}
}

// History returns an iterator over [TaskHistory] entries for the given task ID,
// ordered by start time descending (most recent first). Use slices.Collect to
// materialize the results as a slice when random access is needed.
func (s *Scheduler) History(ctx context.Context, id string) iter.Seq2[*TaskHistory, error] {
	return s.storage.History(ctx, id)
}

// TasksPaginated returns a paginated slice of [TaskSummary] values. Both
// cursor-seek and filter are pushed to the [Storage] layer via
// [Storage.TasksPaginated].
func (s *Scheduler) TasksPaginated(ctx context.Context, page PageRequest, filterExpr string) (*PageResult[*TaskSummary], error) {
	limit := page.ClampLimit()

	// Decode cursor if provided
	var afterID string
	if page.Cursor != "" {
		cur, err := decodeTaskCursor(page.Cursor, filterExpr)
		if err != nil {
			return nil, err
		}
		afterID = cur.LastID
	}

	// Parse filter expression (nil when empty)
	var filterNode filter.Node
	if filterExpr != "" {
		var err error
		filterNode, err = s.filterParser.Parse(ctx, filterExpr)
		if err != nil {
			return nil, err
		}
	}

	pg := Pagination{AfterID: afterID, Limit: limit}
	states, err := s.storage.TasksPaginated(ctx, pg, filterNode)
	if err != nil {
		return nil, err
	}
	return buildTaskPageResult(states, limit, filterExpr), nil
}

// HistoryPaginated returns a paginated slice of [TaskHistory] values. Both
// cursor-seek and filter are pushed to the [Storage] layer via
// [Storage.HistoryPaginated].
func (s *Scheduler) HistoryPaginated(ctx context.Context, taskID string, page PageRequest, filterExpr string) (*PageResult[*TaskHistory], error) {
	limit := page.ClampLimit()

	// Decode cursor if provided
	var (
		afterStartedAt int64
		afterID        string
	)
	if page.Cursor != "" {
		cur, err := decodeHistoryCursor(page.Cursor, filterExpr)
		if err != nil {
			return nil, err
		}
		afterStartedAt = cur.LastStartedAt
		afterID = cur.LastID
	}

	// Parse filter expression (nil when empty)
	var filterNode filter.Node
	if filterExpr != "" {
		var err error
		filterNode, err = s.filterParser.Parse(ctx, filterExpr)
		if err != nil {
			return nil, err
		}
	}

	pg := HistoryPagination{
		Pagination:     Pagination{AfterID: afterID, Limit: limit},
		AfterStartedAt: afterStartedAt,
	}
	entries, err := s.storage.HistoryPaginated(ctx, taskID, pg, filterNode)
	if err != nil {
		return nil, err
	}
	return buildHistoryPageResult(entries, limit, filterExpr), nil
}

// buildTaskPageResult constructs a [PageResult] from a slice of [TaskState]
// returned by [Storage.TasksPaginated]. The slice may contain up to limit+1 items;
// if the extra item is present, it is trimmed and a NextCursor is emitted.
func buildTaskPageResult(states []*TaskState, limit int64, filterExpr string) *PageResult[*TaskSummary] {
	hasMore := int64(len(states)) > limit
	if hasMore {
		states = states[:limit]
	}

	summaries := make([]*TaskSummary, len(states))
	for i, st := range states {
		s := st.TaskSummary
		summaries[i] = &s
	}

	result := &PageResult[*TaskSummary]{Items: summaries}
	if hasMore && len(summaries) > 0 {
		c := encodeTaskCursor(summaries[len(summaries)-1].ID, filterExpr)
		result.NextCursor = &c
	}
	return result
}

// buildHistoryPageResult constructs a [PageResult] from a slice of [TaskHistory].
func buildHistoryPageResult(entries []*TaskHistory, limit int64, filterExpr string) *PageResult[*TaskHistory] {
	hasMore := int64(len(entries)) > limit
	if hasMore {
		entries = entries[:limit]
	}

	result := &PageResult[*TaskHistory]{Items: entries}
	if hasMore && len(entries) > 0 {
		last := entries[len(entries)-1]
		c := encodeHistoryCursor(last.StartedAt, last.ID, filterExpr)
		result.NextCursor = &c
	}
	return result
}

// TriggerTask manually triggers immediate execution of a task in a background
// goroutine. The task's regular schedule is not affected; it will still fire at
// its next scheduled time. The triggered execution respects concurrency limits
// and priority-based slot allocation.
//
// Returns [ErrTaskNotRegistered] if the task ID is not in the in-memory map,
// [ErrTaskNotFound] if the task state does not exist in [Storage],
// [ErrTaskCompleted] if the task is a completed one-shot, or
// [ErrTaskDisabled] if the task's status is [TaskStatusDisabled].
func (s *Scheduler) TriggerTask(ctx context.Context, id string) error {
	s.mu.RLock()
	task, ok := s.tasks[id]
	s.mu.RUnlock()

	if !ok {
		return ErrTaskNotRegistered
	}

	state, err := s.storage.GetTask(ctx, id)
	if err != nil {
		return err
	}

	if state == nil {
		return ErrTaskNotFound
	}

	if state.Status == TaskStatusCompleted {
		return ErrTaskCompleted
	}

	if state.Status == TaskStatusDisabled {
		return ErrTaskDisabled
	}

	// Execute in background
	s.wg.Go(func() {
		s.runTaskWithSemaphore(task, state)
	})

	return nil
}
