// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package buffered provides an asynchronous [slog.Handler] that buffers log
// records and writes them via a background goroutine.
//
// Records at or above the bypass level ([WithBypassLevel], default [slog.LevelError])
// are written synchronously after flushing the buffer, ensuring high-severity
// entries are never lost. When the buffer is full, records fall back to
// synchronous writes rather than being dropped.
//
// Call [Handler.Shutdown] before process exit to drain the buffer. The handler
// also implements [slogx.HandlerWithShutdown], so [slogx.Shutdown] can
// traverse the handler chain automatically.
//
// # Example
//
//	h := buffered.NewHandler(slog.NewJSONHandler(os.Stdout, nil),
//	    buffered.WithBufferSize(200),
//	    buffered.WithBypassLevel(slog.LevelError),
//	)
//	defer h.Shutdown(context.Background())
//	logger := slog.New(h)
package buffered
