// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory maps the go-atlas logger configuration to a slog logger
// assembled by github.com/altessa-s/go-slogx/setup (leveled, masking,
// prefixed, console or JSON sink, optional buffering).
//
//	logger, err := factory.New(cfg.Logger).
//	    WithEnableMasking().
//	    Build()
//
// With buffering enabled, setting buffer.bypassLevel to none disables
// synchronous flushing by log level, including for error records.
//
// # Prefix key
//
// The prefixed and console handlers use [github.com/altessa-s/go-slogx.ModuleKey], which equals
// slogx.ModuleKey ("subsystem"): a logger created with
// logger.With(slogx.Module("auth")) shows "[auth]" as its prefix tag and is
// matched against observabilityconfig.Logger.Subsystems. The default used to be "module";
// call [LoggerBuilder.WithPrefixKey]("module") to keep that behavior.
//
// # Mask types
//
// maskRules types resolve against the go-slogx mask catalog, extended with
// the go-atlas names smart, hash, cc, and cached_partial.
package factory
