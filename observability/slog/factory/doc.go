// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides a fluent builder for creating slog loggers
// from configuration. It integrates colorized, prefixed, and masking
// handlers to create structured loggers with consistent defaults.
//
//	logger, err := factory.New(cfg.Logger).
//	    WithEnableMasking().
//	    Build()
package factory
