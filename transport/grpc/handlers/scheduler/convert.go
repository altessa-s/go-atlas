// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scheduler

import (
	coremaps "github.com/altessa-s/go-atlas/core/collections/maps"
	sched "github.com/altessa-s/go-atlas/service/scheduler"
)

// taskStateToMap converts a TaskState to a map for filter evaluation.
// Field names use proto camelCase to match CEL expressions.
//
//nolint:unused // used by tests; linter does not scan test files (tests: false).
func taskStateToMap(s *sched.TaskState) map[string]any {
	return map[string]any{
		"id":             s.ID,
		"description":    s.Description,
		"status":         int64(s.Status),
		"priority":       int64(s.Priority),
		"schedule":       s.Schedule,
		"lastRunAt":      s.LastRunAt,
		"nextRunAt":      s.NextRunAt,
		"lastRunId":      s.LastRunID,
		"failures":       int64(s.Failures),
		"skipNextRun":    s.SkipNextRun,
		"disableHistory": s.DisableHistory,
		"unmanaged":      s.Unmanaged,
		"meta":           coremaps.ConvertMap(s.Meta, func(k, v string) (string, any) { return k, v }),
		"createdAt":      s.CreatedAt,
		"updatedAt":      s.UpdatedAt,
	}
}

// taskSummaryToMap converts a TaskSummary to a map for filter evaluation.
func taskSummaryToMap(s *sched.TaskSummary) map[string]any {
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
		"failures":       int64(s.Failures),
	}
}

// taskHistoryToMap converts a TaskHistory to a map for filter evaluation.
func taskHistoryToMap(s *sched.TaskHistory) map[string]any {
	return map[string]any{
		"id":         s.ID,
		"taskId":     s.TaskID,
		"runId":      s.RunID,
		"startedAt":  s.StartedAt,
		"endedAt":    s.EndedAt,
		"durationMs": s.DurationMs,
		"success":    s.Success,
		"error":      s.Error,
	}
}
