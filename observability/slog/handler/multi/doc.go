// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package multi provides a [slog.Handler] that fans out log records to
// multiple child handlers. Each record is dispatched to every child whose
// Enabled method returns true for the record's level.
//
// On Go 1.26+ this delegates to [slog.MultiHandler] from the standard library.
//
// The handler is safe for concurrent use and immutable after creation —
// [Handler.WithAttrs] and [Handler.WithGroup] return new instances.
//
// The handler implements [slogx.InnerHandlers], so [slogx.Shutdown] can
// traverse all children automatically.
//
// # Example
//
//	h := multi.NewHandler(
//	    slog.NewJSONHandler(os.Stdout, nil),
//	    slog.NewTextHandler(logFile, nil),
//	)
//	logger := slog.New(h)
//
// For handlers that may block (e.g. network loggers), use
// [NewConcurrentHandler] to dispatch records concurrently:
//
//	h := multi.NewConcurrentHandler(
//	    slog.NewJSONHandler(os.Stdout, nil),
//	    networkHandler,
//	)
//	logger := slog.New(h)
package multi
