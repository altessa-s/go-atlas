// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package meilisearch

import (
	"context"
	"log/slog"
	"time"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	msdk "github.com/meilisearch/meilisearch-go"
)

// WaitForTask blocks until the Meilisearch task identified by taskUID reaches
// a terminal state, polling the server every interval (pass 0 to use the
// SDK's default cadence). It is the completion half of the fire-and-forget
// write methods: [Client.IndexDocuments], [Client.DeleteDocuments], and
// [Client.SwapIndexes] all return a task UID that this method awaits.
//
// Returns nil only when the task finished as "succeeded". A task that
// finished in any other terminal state (e.g. "failed") yields an error
// wrapping [ErrTaskFailed] — match it with [errors.Is] — carrying the task
// status and the Meilisearch error code/message. A canceled or expired ctx
// aborts the wait and returns the underlying SDK error.
func (c *Client) WaitForTask(ctx context.Context, taskUID int64, interval time.Duration) error {
	task, err := c.sdk.WaitForTaskWithContext(ctx, taskUID, interval)
	if err != nil {
		return coreerrs.Wrapf(err, "wait for task %d", taskUID)
	}

	if task.Status != msdk.TaskStatusSucceeded {
		return coreerrs.Wrapf(ErrTaskFailed,
			"task %d finished with status %q (code %q: %s)",
			taskUID, task.Status, task.Error.Code, task.Error.Message)
	}

	c.logger.DebugContext(ctx, "task completed",
		slog.Int64("task_uid", taskUID),
		slog.String("status", string(task.Status)))

	return nil
}
