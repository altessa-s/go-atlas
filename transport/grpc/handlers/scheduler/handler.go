// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scheduler

import (
	"context"

	"github.com/altessa-s/go-atlas/data/filter"
	"github.com/altessa-s/go-atlas/domain/converter"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	schedulerv1 "github.com/altessa-s/go-atlas/proto/gen/scheduler/v1"
	sched "github.com/altessa-s/go-atlas/service/scheduler"
	stdGrpc "google.golang.org/grpc"
)

// Handler implements schedulerv1.SchedulerServiceServer by delegating to
// a [sched.Scheduler]. It owns a CEL filter [filter.Parser] and
// [filter.Evaluator] for client-side list filtering.
type Handler struct {
	schedulerv1.UnimplementedSchedulerServiceServer

	scheduler *sched.Scheduler
	parser    *filter.Parser
	evaluator *filter.Evaluator
}

// New constructs a Handler backed by s. Returns an error if the internal
// CEL filter parser cannot be initialized.
func New(s *sched.Scheduler) (*Handler, error) {
	p, err := filter.NewParser()
	if err != nil {
		return nil, coreerrs.Wrap(err, "creating filter parser")
	}
	return &Handler{
		scheduler: s,
		parser:    p,
		evaluator: filter.NewEvaluator(),
	}, nil
}

// Register attaches the handler to the given gRPC server instance.
func (h *Handler) Register(gs *stdGrpc.Server, _ <-chan struct{}) {
	schedulerv1.RegisterSchedulerServiceServer(gs, h)
}

// Get returns the full state of a task.
func (h *Handler) Get(ctx context.Context, req *schedulerv1.TaskGetRequest) (*schedulerv1.TaskGetResponse, error) {
	state, err := h.scheduler.GetTaskState(ctx, req.GetTaskId())
	if err != nil {
		return nil, mapError(err)
	}
	if state == nil {
		return nil, mapError(sched.ErrTaskNotFound)
	}

	return &schedulerv1.TaskGetResponse{TaskState: &schedulerv1.TaskState{
		Summary:   converter.Convert(&state.TaskSummary, &schedulerv1.TaskSummary{}),
		LastRunId: state.LastRunID,
		Meta:      state.Meta,
		CreatedAt: state.CreatedAt,
		UpdatedAt: state.UpdatedAt,
	}}, nil
}

// List returns summaries of all tasks, optionally filtered by a CEL expression
// in req.Filter. Server-side filtering is attempted first; if the storage
// backend does not support it, filtering falls back to in-memory evaluation.
// Returns codes.InvalidArgument for malformed filter expressions.
func (h *Handler) List(ctx context.Context, req *schedulerv1.TasksListRequest) (*schedulerv1.TasksListResponse, error) {
	var filterNode filter.Node
	if f := req.GetFilter(); f != "" {
		var err error
		filterNode, err = h.parser.Parse(f) //nolint:contextcheck // parser is a pure function, no context needed
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid filter: %v", err)
		}
	}

	// Try server-side filtering if storage supports it
	if filterNode != nil {
		if seq, ok := h.scheduler.TasksFiltered(ctx, filterNode); ok {
			tasks := make([]*schedulerv1.TaskSummary, 0)
			for summary, err := range seq {
				if err != nil {
					return nil, mapError(err)
				}
				tasks = append(tasks, converter.Convert(summary, &schedulerv1.TaskSummary{}))
			}
			return &schedulerv1.TasksListResponse{Tasks: tasks}, nil
		}
	}

	// Fallback: client-side filtering
	tasks := make([]*schedulerv1.TaskSummary, 0)
	for summary, err := range h.scheduler.Tasks(ctx) {
		if err != nil {
			return nil, mapError(err)
		}

		if filterNode != nil {
			match, err := h.evaluator.Evaluate(filterNode, taskSummaryToMap(summary))
			if err != nil {
				return nil, status.Errorf(codes.InvalidArgument, "filter evaluation failed: %v", err)
			}
			if !match {
				continue
			}
		}

		tasks = append(tasks, converter.Convert(summary, &schedulerv1.TaskSummary{}))
	}
	return &schedulerv1.TasksListResponse{Tasks: tasks}, nil
}

// ListHistory returns the execution history for a task, optionally filtered
// by a CEL expression. Uses the same server-side-then-client-side filtering
// strategy as [Handler.List]. Returns codes.InvalidArgument for invalid filters.
func (h *Handler) ListHistory(ctx context.Context, req *schedulerv1.TaskHistoryListRequest) (*schedulerv1.TaskHistoryListResponse, error) {
	var filterNode filter.Node
	if f := req.GetFilter(); f != "" {
		var err error
		filterNode, err = h.parser.Parse(f) //nolint:contextcheck // parser is a pure function, no context needed
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid filter: %v", err)
		}
	}

	// Try server-side filtering if storage supports it
	if filterNode != nil {
		if seq, ok := h.scheduler.HistoryFiltered(ctx, req.GetTaskId(), filterNode); ok {
			entries := make([]*schedulerv1.TaskHistory, 0)
			for history, err := range seq {
				if err != nil {
					return nil, mapError(err)
				}
				entries = append(entries, converter.Convert(history, &schedulerv1.TaskHistory{}))
			}
			return &schedulerv1.TaskHistoryListResponse{Entries: entries}, nil
		}
	}

	// Fallback: client-side filtering
	entries := make([]*schedulerv1.TaskHistory, 0)
	for history, err := range h.scheduler.History(ctx, req.GetTaskId()) {
		if err != nil {
			return nil, mapError(err)
		}

		if filterNode != nil {
			match, err := h.evaluator.Evaluate(filterNode, taskHistoryToMap(history))
			if err != nil {
				return nil, status.Errorf(codes.InvalidArgument, "filter evaluation failed: %v", err)
			}
			if !match {
				continue
			}
		}

		entries = append(entries, converter.Convert(history, &schedulerv1.TaskHistory{}))
	}
	return &schedulerv1.TaskHistoryListResponse{Entries: entries}, nil
}

// GetStatus returns a snapshot of the scheduler's runtime state including
// leader status, running task counts, and available concurrency slots.
func (h *Handler) GetStatus(_ context.Context, _ *schedulerv1.SchedulerStatusGetRequest) (*schedulerv1.SchedulerStatusGetResponse, error) {
	return &schedulerv1.SchedulerStatusGetResponse{
		IsRunning:                  h.scheduler.IsRunning(),
		IsLeader:                   h.scheduler.IsLeader(),
		RunningTasksCount:          int32(h.scheduler.RunningTasksCount()),          // #nosec G115 -- bounded by maxConcurrency
		AvailableSlots:             int32(h.scheduler.AvailableSlots()),             // #nosec G115 -- bounded by maxConcurrency
		AvailableHighPrioritySlots: int32(h.scheduler.AvailableHighPrioritySlots()), // #nosec G115 -- bounded by maxConcurrency
	}, nil
}

// Pause pauses a task.
func (h *Handler) Pause(ctx context.Context, req *schedulerv1.TaskPauseRequest) (*schedulerv1.TaskPauseResponse, error) {
	if err := h.scheduler.PauseTask(ctx, req.GetTaskId()); err != nil {
		return nil, mapError(err)
	}
	return &schedulerv1.TaskPauseResponse{}, nil
}

// Resume resumes a paused task.
func (h *Handler) Resume(ctx context.Context, req *schedulerv1.TaskResumeRequest) (*schedulerv1.TaskResumeResponse, error) {
	if err := h.scheduler.ResumeTask(ctx, req.GetTaskId()); err != nil {
		return nil, mapError(err)
	}
	return &schedulerv1.TaskResumeResponse{}, nil
}

// Disable disables a task.
func (h *Handler) Disable(ctx context.Context, req *schedulerv1.TaskDisableRequest) (*schedulerv1.TaskDisableResponse, error) {
	if err := h.scheduler.DisableTask(ctx, req.GetTaskId()); err != nil {
		return nil, mapError(err)
	}
	return &schedulerv1.TaskDisableResponse{}, nil
}

// Enable enables a disabled task.
func (h *Handler) Enable(ctx context.Context, req *schedulerv1.TaskEnableRequest) (*schedulerv1.TaskEnableResponse, error) {
	if err := h.scheduler.EnableTask(ctx, req.GetTaskId()); err != nil {
		return nil, mapError(err)
	}
	return &schedulerv1.TaskEnableResponse{}, nil
}

// SkipNextRun marks the next run of a task to be skipped.
func (h *Handler) SkipNextRun(ctx context.Context, req *schedulerv1.TaskSkipNextRunRequest) (*schedulerv1.TaskSkipNextRunResponse, error) {
	if err := h.scheduler.SkipNextRun(ctx, req.GetTaskId()); err != nil {
		return nil, mapError(err)
	}
	return &schedulerv1.TaskSkipNextRunResponse{}, nil
}

// Trigger manually triggers immediate execution of a task, bypassing its
// cron schedule. Returns codes.FailedPrecondition if the task is in a state
// that does not allow triggering (e.g. disabled or completed).
func (h *Handler) Trigger(ctx context.Context, req *schedulerv1.TaskTriggerRequest) (*schedulerv1.TaskTriggerResponse, error) {
	if err := h.scheduler.TriggerTask(ctx, req.GetTaskId()); err != nil {
		return nil, mapError(err)
	}
	return &schedulerv1.TaskTriggerResponse{}, nil
}
