// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scheduler

import (
	"context"

	"github.com/altessa-s/go-aip/filter"
	"github.com/altessa-s/go-aip/pagination/pagetoken"
	"github.com/altessa-s/go-horai/scheduler"

	"github.com/altessa-s/go-atlas-core/types/nilcheck"

	schedulerv1 "github.com/altessa-s/go-atlas/proto/gen/scheduler/v1"
	converter "github.com/altessa-s/go-convert"
	horai "github.com/altessa-s/go-horai"
	stdGrpc "google.golang.org/grpc"
)

// Handler implements [schedulerv1.SchedulerServiceServer] by delegating to a
// go-horai [scheduler.Scheduler]. It owns what the scheduler leaves to the
// transport: parsing CEL filters into typed queries and signing page tokens.
type Handler struct {
	schedulerv1.UnimplementedSchedulerServiceServer

	scheduler *scheduler.Scheduler
	tokens    TokenCodec
	parser    FilterParser
}

// TokenCodec issues and resolves bound scheduler page tokens.
type TokenCodec interface {
	Issue(payload []byte, b pagetoken.Bindings) (string, error)
	Resolve(token string, b pagetoken.Bindings) ([]byte, error)
}

// FilterParser parses filter expressions into the filter AST.
type FilterParser interface {
	Parse(context.Context, string) (filter.Node, error)
}

// New creates a Handler serving s. tokens signs the cursors of List and
// ListHistory; use one codec with the same key on every instance serving the
// lists. Without it (nil) both RPCs fail with FailedPrecondition
// ([ErrPageTokensNotConfigured]).
func New(s *scheduler.Scheduler, tokens TokenCodec, parser FilterParser) (*Handler, error) {
	if s == nil {
		return nil, ErrSchedulerRequired
	}
	if nilcheck.IsNil(parser) {
		return nil, ErrFilterParserRequired
	}
	if nilcheck.IsNil(tokens) {
		tokens = nil
	}
	return &Handler{scheduler: s, tokens: tokens, parser: parser}, nil
}

// Register registers the handler with gs.
func (h *Handler) Register(gs *stdGrpc.Server, _ <-chan struct{}) {
	schedulerv1.RegisterSchedulerServiceServer(gs, h)
}

// Get returns the full state of a task.
func (h *Handler) Get(ctx context.Context, req *schedulerv1.TaskGetRequest) (*schedulerv1.TaskGetResponse, error) {
	state, err := h.scheduler.Task(ctx, req.GetTaskId())
	if err != nil {
		return nil, mapError(err)
	}
	if state == nil {
		return nil, mapError(horai.ErrTaskNotFound)
	}

	return &schedulerv1.TaskGetResponse{TaskState: &schedulerv1.TaskState{
		Summary:   converter.Convert(&state.TaskSummary, &schedulerv1.TaskSummary{}),
		LastRunId: state.LastRunID,
		Meta:      state.Meta,
		CreatedAt: state.CreatedAt,
		UpdatedAt: state.UpdatedAt,
	}}, nil
}

// List returns a page of task summaries matching the optional CEL filter.
func (h *Handler) List(ctx context.Context, req *schedulerv1.TasksListRequest) (*schedulerv1.TasksListResponse, error) {
	if h.tokens == nil {
		return nil, mapError(ErrPageTokensNotConfigured)
	}
	after, err := h.decodeTaskCursor(req.GetCursor(), req.GetFilter())
	if err != nil {
		return nil, mapError(err)
	}
	expr, err := h.parseFilter(ctx, req.GetFilter(), taskFields)
	if err != nil {
		return nil, mapError(err)
	}

	page, err := h.scheduler.ListTasks(ctx, scheduler.TaskListRequest{After: after, Limit: pageSize(req.GetLimit()), Filter: expr})
	if err != nil {
		return nil, mapError(err)
	}

	tasks := make([]*schedulerv1.TaskSummary, len(page.Items))
	for i, summary := range page.Items {
		tasks[i] = converter.Convert(summary, &schedulerv1.TaskSummary{})
	}
	next, err := h.encodeTaskCursor(page.Next, req.GetFilter())
	if err != nil {
		return nil, mapError(err)
	}

	return &schedulerv1.TasksListResponse{Tasks: tasks, NextCursor: next}, nil
}

// ListHistory returns a page of a task's execution history matching the
// optional CEL filter.
func (h *Handler) ListHistory(ctx context.Context, req *schedulerv1.TaskHistoryListRequest) (*schedulerv1.TaskHistoryListResponse, error) {
	if h.tokens == nil {
		return nil, mapError(ErrPageTokensNotConfigured)
	}
	after, err := h.decodeHistoryCursor(req.GetCursor(), req.GetTaskId(), req.GetFilter())
	if err != nil {
		return nil, mapError(err)
	}
	expr, err := h.parseFilter(ctx, req.GetFilter(), historyFields)
	if err != nil {
		return nil, mapError(err)
	}

	page, err := h.scheduler.ListHistory(ctx, scheduler.HistoryListRequest{
		TaskID: req.GetTaskId(), After: after, Limit: pageSize(req.GetLimit()), Filter: expr,
	})
	if err != nil {
		return nil, mapError(err)
	}

	entries := make([]*schedulerv1.TaskHistory, len(page.Items))
	for i, entry := range page.Items {
		entries[i] = converter.Convert(entry, &schedulerv1.TaskHistory{})
	}
	next, err := h.encodeHistoryCursor(page.Next, req.GetTaskId(), req.GetFilter())
	if err != nil {
		return nil, mapError(err)
	}

	return &schedulerv1.TaskHistoryListResponse{Entries: entries, NextCursor: next}, nil
}

// GetStatus returns the scheduler's runtime state.
func (h *Handler) GetStatus(_ context.Context, _ *schedulerv1.SchedulerStatusGetRequest) (*schedulerv1.SchedulerStatusGetResponse, error) {
	st := h.scheduler.Status()
	return &schedulerv1.SchedulerStatusGetResponse{
		IsRunning:                  st.Running,
		IsLeader:                   st.Leader,
		RunningTasksCount:          int32(st.RunningTasks),               // #nosec G115 -- bounded by the concurrency limit
		AvailableSlots:             int32(st.AvailableSlots),             // #nosec G115 -- bounded by the concurrency limit
		AvailableHighPrioritySlots: int32(st.AvailableHighPrioritySlots), // #nosec G115 -- bounded by the concurrency limit
	}, nil
}

// Pause pauses a task.
func (h *Handler) Pause(ctx context.Context, req *schedulerv1.TaskPauseRequest) (*schedulerv1.TaskPauseResponse, error) {
	if err := h.scheduler.Pause(ctx, req.GetTaskId()); err != nil {
		return nil, mapError(err)
	}
	return &schedulerv1.TaskPauseResponse{}, nil
}

// Resume resumes a paused task.
func (h *Handler) Resume(ctx context.Context, req *schedulerv1.TaskResumeRequest) (*schedulerv1.TaskResumeResponse, error) {
	if err := h.scheduler.Resume(ctx, req.GetTaskId()); err != nil {
		return nil, mapError(err)
	}
	return &schedulerv1.TaskResumeResponse{}, nil
}

// Disable disables a task.
func (h *Handler) Disable(ctx context.Context, req *schedulerv1.TaskDisableRequest) (*schedulerv1.TaskDisableResponse, error) {
	if err := h.scheduler.Disable(ctx, req.GetTaskId()); err != nil {
		return nil, mapError(err)
	}
	return &schedulerv1.TaskDisableResponse{}, nil
}

// Enable enables a disabled task.
func (h *Handler) Enable(ctx context.Context, req *schedulerv1.TaskEnableRequest) (*schedulerv1.TaskEnableResponse, error) {
	if err := h.scheduler.Enable(ctx, req.GetTaskId()); err != nil {
		return nil, mapError(err)
	}
	return &schedulerv1.TaskEnableResponse{}, nil
}

// SkipNextRun marks the next scheduled run of a task to be skipped.
func (h *Handler) SkipNextRun(ctx context.Context, req *schedulerv1.TaskSkipNextRunRequest) (*schedulerv1.TaskSkipNextRunResponse, error) {
	if err := h.scheduler.SkipNextRun(ctx, req.GetTaskId()); err != nil {
		return nil, mapError(err)
	}
	return &schedulerv1.TaskSkipNextRunResponse{}, nil
}

// Trigger runs a task immediately, outside its schedule.
func (h *Handler) Trigger(ctx context.Context, req *schedulerv1.TaskTriggerRequest) (*schedulerv1.TaskTriggerResponse, error) {
	if err := h.scheduler.Trigger(ctx, req.GetTaskId()); err != nil {
		return nil, mapError(err)
	}
	return &schedulerv1.TaskTriggerResponse{}, nil
}
