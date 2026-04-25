// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package leveled provides a slog.Handler that filters log records based on
// per-subsystem log levels. It uses the "subsystem" attribute (see [slogx.ModuleKey])
// to determine which level threshold applies.
//
// When a subsystem is known at logger creation time (via [slog.Logger.With]),
// Enabled performs a single integer comparison (zero-cost fast path).
// Otherwise, Handle scans record attributes to find the subsystem.
//
// Example:
//
//	handler := leveled.NewHandler(next,
//	    leveled.WithDefaultLevel(slog.LevelInfo),
//	    leveled.WithSubsystemLevels(map[string]slog.Level{
//	        "scheduler": slog.LevelDebug,
//	    }),
//	)
//	logger := slog.New(handler)
package leveled
