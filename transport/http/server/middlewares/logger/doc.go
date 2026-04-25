// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package logger provides HTTP middleware for structured request/response logging.
// It supports configurable filtering and integrates with the realip middleware
// for accurate client IP logging.
//
// Example with realip middleware:
//
//	ipExtractor := clientip.NewExtractor()
//	router.Use(
//	    realip.Middleware(ipExtractor),
//	    logger.Middleware(logger.Slog(slog.Default())),
//	)
//
// Example standalone:
//
//	mw := logger.Middleware(logger.Slog(slog.Default()),
//	    logger.WithIgnorePaths("/health", "/metrics"),
//	)
package logger
