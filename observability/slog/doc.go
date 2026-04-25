// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package slog provides nil-safe attribute helpers, context integration, and advanced handlers
// for Go's log/slog. It simplifies structured logging with pointer types and adds features
// like colorized output and attribute masking.
//
// # Features
//
//   - Nil-safe attributes: helpers like String, Int, etc., that safely handle nil pointers.
//   - Context Integration: easy storage and retrieval of loggers in context.
//   - Advanced Handlers: built-in support for colorized consoles, prefixed logs, and PII masking.
//   - Configuration: dynamic log level management and structured output formatting.
//
// # Usage
//
// Import with alias to avoid conflict with standard log/slog:
//
//	import slogx "github.com/altessa-s/go-atlas/observability/slog"
//
//	var name *string = getName()
//	logger.Info("event", slogx.String("name", name), slogx.Error(err))
//
//	// Context usage
//	ctx = slogx.ContextWithLogger(ctx, logger)
//	logger = slogx.FromContext(ctx)
package slog
