// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides a fluent builder for creating tracers
// from configuration.
//
// Returns [tracing.Noop] when config is nil or tracing is disabled.
//
//	tracer, err := factory.New(cfg.Tracing).
//	    UseLogger(logger).
//	    Build(ctx)
package factory
