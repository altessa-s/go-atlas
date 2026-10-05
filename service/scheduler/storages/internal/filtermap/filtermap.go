// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filtermap

import "github.com/altessa-s/go-atlas/service/scheduler"

// Task converts a task state to a map for filter evaluation. Field names use
// proto camelCase to match CEL expressions.
func Task(s *scheduler.TaskState) map[string]any {
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

// History converts a history entry to a map for filter evaluation.
func History(h *scheduler.TaskHistory) map[string]any {
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
