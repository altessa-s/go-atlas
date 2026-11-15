// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package bloom

//go:generate go tool optgen generate

import (
	"log/slog"

	"github.com/altessa-s/go-atlas/cache/probfilter"

	schedtask "github.com/altessa-s/go-horai/task"
)

// DefaultName is the filter name used in the rebuild task ID and in log
// records when [WithName] is not set.
const DefaultName = "bloom"

// RebuildTaskPrefix prefixes the scheduler task ID of a filter rebuild; the
// filter name ([WithName]) completes it.
const RebuildTaskPrefix = "probfilter-rebuild-"

// options contains Bloom filter configuration. Every setting only affects
// [Filter.StartRebuild]; a filter that never calls it has no rebuild
// lifecycle.
type options struct {
	// name identifies the filter in the rebuild task ID and in log records.
	name string `optgen:"default=DefaultName"`

	// logger receives the rebuild lifecycle records.
	logger *slog.Logger `optgen:"default=slog.New(slog.DiscardHandler),notnil"`

	// rebuildLoader is the data source of the rebuild lifecycle; without one
	// StartRebuild is inert.
	rebuildLoader probfilter.DataLoader `optgen:"notnil"`

	// rebuildOnStart rebuilds the filter synchronously inside StartRebuild.
	rebuildOnStart bool

	// rebuildSchedule is the cron expression of periodic rebuilds; empty
	// disables them.
	rebuildSchedule string

	// tolerateRebuildInProgress lets the initial rebuild of a shared filter
	// lose the rebuild lease to another process instead of failing.
	tolerateRebuildInProgress bool

	// scheduler runs the periodic rebuild of a shared filter. A process-local
	// filter never uses it.
	scheduler schedtask.Registrar `optgen:"notnil"`
}
