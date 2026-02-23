// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides configuration-based creation of slog loggers.
// It integrates colorized, prefixed, and masking handlers to create structured
// loggers with consistent defaults.
//
// Example:
//
//	f := factory.New()
//	logger := f.CreateLoggerFromConfig(cfg)
//	logger.Info("Application started")
package factory
