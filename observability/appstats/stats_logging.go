// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package appstats

import (
	"context"
	"log/slog"
	"time"

	corectx "github.com/altessa-s/go-atlas/core/context"
)

const (
	// DefaultStatsLogSchedule is the default cron schedule for periodic stats logging.
	// Default: every 5 minutes.
	DefaultStatsLogSchedule = "0 */5 * * * *"

	// DefaultStatsCollectionTimeout is the default timeout for collecting application statistics.
	DefaultStatsCollectionTimeout = 2 * time.Second
)

// StatsLogger provides periodic logging of application statistics.
// It logs CPU load, memory usage, network I/O, goroutine count, and uptime.
//
// For periodic logging, register RunLogCycle with service/scheduler:
//
//	statsLogger := appstats.NewStatsLogger(appstats.WithLogger(logger))
//	sched.Register(ctx, scheduler.TaskConfig{
//	    ID:         "appstats-logging",
//	    Schedule:   appstats.DefaultStatsLogSchedule,
//	    Func:       statsLogger.RunLogCycle,
//	    RunOnStart: true,
//	})
type StatsLogger struct {
	logger *slog.Logger
}

// StatsLoggerOption configures StatsLogger behavior.
type StatsLoggerOption func(*StatsLogger)

// WithLogger sets a custom logger for stats logging.
// If nil, slog.Default() is used.
func WithLogger(logger *slog.Logger) StatsLoggerOption {
	return func(s *StatsLogger) {
		if logger != nil {
			s.logger = logger
		}
	}
}

// NewStatsLogger creates a new StatsLogger with the given options.
func NewStatsLogger(opts ...StatsLoggerOption) *StatsLogger {
	s := &StatsLogger{
		logger: slog.Default(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// RunLogCycle performs a single logging cycle of application statistics.
// This method is designed to be registered with service/scheduler for periodic execution.
//
// It collects and logs CPU load, memory usage, network I/O, goroutine count, and uptime.
// The context controls the timeout for metrics collection (DefaultStatsCollectionTimeout).
//
// Returns nil on success. Errors during metrics collection are logged but not returned,
// as partial statistics are still valuable.
func (s *StatsLogger) RunLogCycle(ctx context.Context) error {
	ctx, cancel := corectx.ApplyTimeout(ctx, DefaultStatsCollectionTimeout)
	defer cancel()

	// Get comprehensive application statistics
	stats := GetApplicationStats(ctx)

	// Prepare log fields grouped by category
	logFields := []slog.Attr{
		// Process information
		slog.Group("process",
			slog.Int("id", int(stats.ProcessID)),
			slog.Duration("uptime", stats.Uptime),
		),

		// CPU statistics
		slog.Group("cpu",
			slog.String("service", stats.CPU.Service),
			slog.String("system", stats.CPU.System),
		),

		// Memory statistics
		slog.Group("memory",
			slog.String("service", stats.Memory.Service),
			slog.String("system", stats.Memory.System),
			slog.String("total", stats.Memory.Total),
		),

		// Network I/O statistics
		slog.Group("network",
			slog.String("bytes_received", stats.Network.BytesReceived),
			slog.String("bytes_sent", stats.Network.BytesSent),
			slog.Float64("raw_received", stats.Network.RawReceived),
			slog.Float64("raw_sent", stats.Network.RawSent),
		),

		// Go runtime statistics
		slog.Group("runtime",
			slog.Int("goroutines", stats.Runtime.Goroutines),
			slog.Group("heap",
				slog.Uint64("alloc_bytes", stats.Runtime.HeapAllocBytes),
				slog.Uint64("sys_bytes", stats.Runtime.HeapSysBytes),
				slog.Uint64("idle_bytes", stats.Runtime.HeapIdleBytes),
				slog.Uint64("in_use_bytes", stats.Runtime.HeapInUseBytes),
				slog.Uint64("released_bytes", stats.Runtime.HeapReleasedBytes),
				slog.Uint64("objects", stats.Runtime.HeapObjects),
			),
			slog.Group("gc",
				slog.Uint64("cycles", uint64(stats.Runtime.GCCycles)),
				slog.Uint64("pause_total_ns", stats.Runtime.GCPauseTotalNs),
			),
			slog.Group("stack",
				slog.Uint64("in_use_bytes", stats.Runtime.StackInUseBytes),
				slog.Uint64("sys_bytes", stats.Runtime.StackSysBytes),
			),
		),
	}

	// Add error information if present
	var errorAttrs []any
	if stats.Errors.CPUError != "" {
		errorAttrs = append(errorAttrs, slog.String("cpu", stats.Errors.CPUError))
	}
	if stats.Errors.MemoryError != "" {
		errorAttrs = append(errorAttrs, slog.String("memory", stats.Errors.MemoryError))
	}
	if stats.Errors.NetworkError != "" {
		errorAttrs = append(errorAttrs, slog.String("network", stats.Errors.NetworkError))
	}

	if len(errorAttrs) > 0 {
		logFields = append(logFields, slog.Group("errors", errorAttrs...))
	}

	// Log comprehensive application statistics
	s.logger.LogAttrs(ctx, slog.LevelDebug, "statistics", logFields...)

	return nil
}
