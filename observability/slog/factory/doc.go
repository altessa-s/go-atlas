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
//
// # Prefix key
//
// The prefixed and colorized handlers use [ModuleKey], which equals
// slogx.ModuleKey ("subsystem"): a logger created with
// logger.With(slogx.Module("auth")) shows "[auth]" as its prefix tag and is
// matched against config.Logger.Subsystems. The default used to be "module";
// call [LoggerBuilder.WithPrefixKey]("module") to keep that behavior.
package factory
