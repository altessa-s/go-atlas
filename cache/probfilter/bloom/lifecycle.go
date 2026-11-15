// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package bloom

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/robfig/cron/v3"

	"github.com/altessa-s/go-horai/schedule"

	"github.com/altessa-s/go-atlas-core/types/nilcheck"
	"github.com/altessa-s/go-atlas/cache/probfilter"
	"github.com/altessa-s/go-atlas/cache/probfilter/bloom/storages"

	schedtask "github.com/altessa-s/go-horai/task"
)

// ErrInvalidRebuildSchedule is returned by [Filter.StartRebuild] when the
// rebuild schedule ([WithRebuildSchedule]) is not a valid cron expression.
var ErrInvalidRebuildSchedule = errors.New("invalid rebuild schedule")

// ErrRebuildAlreadyStarted is returned by [Filter.StartRebuild] once it has
// succeeded: the rebuild lifecycle is started at most once.
var ErrRebuildAlreadyStarted = errors.New("filter rebuild lifecycle already started")

// StartRebuild starts the rebuild lifecycle configured by the rebuild options.
// [New] has no side effects; a filter whose owner drives [Filter.Rebuild]
// itself never calls StartRebuild.
//
// Without a data loader ([WithRebuildLoader]) the rebuild settings are inert.
// With [WithRebuildOnStart] the filter is rebuilt synchronously on ctx, so it
// is populated when StartRebuild returns; an initial rebuild refused because
// another process is rebuilding a shared filter fails unless
// [WithTolerateRebuildInProgress] was set.
//
// The rebuild schedule ([WithRebuildSchedule]) runs where the filter's
// contents live. A process-local storage gets a process-local cron that skips
// ticks while a rebuild still runs, logs failed rebuilds and stops when the
// filter is closed: a shared scheduler that dispatches on one node would leave
// the other nodes' filters stale. A shared storage ([storages.ExclusiveRebuilder],
// Redis) is registered once with the scheduler ([WithScheduler]), which may be
// distributed; its task reports rebuild errors and is a no-op once the filter is
// closed or while another process rebuilds it. Without a scheduler the schedule
// of a shared filter is logged as ignored. Scheduled rebuilds do not inherit ctx.
//
// A failed StartRebuild leaves the filter open; the caller decides whether to
// close it.
func (f *Filter) StartRebuild(ctx context.Context) error {
	f.lifecycleMu.Lock()
	defer f.lifecycleMu.Unlock()
	if f.lifecycleStarted {
		return ErrRebuildAlreadyStarted
	}

	o := f.opts
	if nilcheck.IsNil(o.rebuildLoader) {
		o.logger.Debug("probfilter: no data loader, rebuild settings are inert", slog.String("filter", o.name))
		return nil
	}

	if err := f.initialRebuild(ctx); err != nil {
		return err
	}
	if o.rebuildSchedule == "" {
		f.lifecycleStarted = true
		return nil
	}

	var err error
	if _, shared := f.storage.(storages.ExclusiveRebuilder); shared {
		err = f.registerSharedRebuild(ctx)
	} else {
		// The local cron outlives StartRebuild; its ticks run on their own contexts.
		err = f.startLocalRebuild() //nolint:contextcheck // see above
	}
	if err != nil {
		return err
	}
	f.lifecycleStarted = true
	return nil
}

// initialRebuild applies [WithRebuildOnStart].
func (f *Filter) initialRebuild(ctx context.Context) error {
	o := f.opts
	if !o.rebuildOnStart {
		return nil
	}
	err := f.Rebuild(ctx, o.rebuildLoader)
	switch {
	case err == nil:
		return nil
	case o.tolerateRebuildInProgress && errors.Is(err, probfilter.ErrRebuildInProgress):
		// The peer's rebuild publishes a snapshot; the filter stays
		// unpopulated until then, which the caller tolerates.
		o.logger.Info("probfilter: initial rebuild skipped, another process is rebuilding the shared filter",
			slog.String("filter", o.name))
		return nil
	default:
		return fmt.Errorf("initial rebuild of filter %s: %w", o.name, err)
	}
}

// registerSharedRebuild registers the periodic rebuild of a shared filter
// with the scheduler.
func (f *Filter) registerSharedRebuild(ctx context.Context) error {
	o := f.opts
	if nilcheck.IsNil(o.scheduler) {
		o.logger.Warn("probfilter: rebuild schedule ignored, no scheduler injected for a shared filter",
			slog.String("filter", o.name), slog.String("rebuildSchedule", o.rebuildSchedule))
		return nil
	}
	loader := o.rebuildLoader
	err := o.scheduler.Register(ctx, schedtask.Config{
		ID:          RebuildTaskPrefix + o.name,
		Description: "Rebuild probabilistic filter " + o.name + " from its data source",
		Schedule:    o.rebuildSchedule,
		Priority:    schedtask.PriorityNormal,
		Func: func(ctx context.Context) error {
			// A closed filter has been discarded, and a rebuild in progress on
			// another node publishes a fresh snapshot: nothing to do then.
			err := f.Rebuild(ctx, loader)
			if err != nil && !errors.Is(err, probfilter.ErrFilterClosed) && !errors.Is(err, probfilter.ErrRebuildInProgress) {
				return err
			}
			return nil
		},
	})
	if err != nil {
		return fmt.Errorf("register rebuild task of filter %s: %w", o.name, err)
	}
	return nil
}

// startLocalRebuild rebuilds a process-local filter on a cron that is stopped
// as soon as the filter is closed.
func (f *Filter) startLocalRebuild() error {
	o := f.opts
	sched, err := schedule.Parse(o.rebuildSchedule)
	if err != nil {
		return fmt.Errorf("%w %q of filter %s: %w", ErrInvalidRebuildSchedule, o.rebuildSchedule, o.name, err)
	}

	// Skip ticks while a rebuild still runs, so a slow loader cannot pile up
	// queued rebuilds.
	c := cron.New(cron.WithChain(cron.SkipIfStillRunning(cron.DiscardLogger)))
	logger, name, loader := o.logger, o.name, o.rebuildLoader
	c.Schedule(sched, cron.FuncJob(func() {
		err := f.Rebuild(context.Background(), loader)
		if err != nil && !errors.Is(err, probfilter.ErrFilterClosed) {
			logger.Warn("probfilter: scheduled rebuild failed", slog.String("filter", name), slog.Any("error", err))
		}
	}))
	c.Start()
	go func() {
		<-f.Done()
		c.Stop()
	}()
	return nil
}
