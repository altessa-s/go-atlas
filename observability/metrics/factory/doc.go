// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides a fluent builder for creating metrics collectors
// from configuration.
//
// Returns [metrics.Noop] when config is nil or metrics are disabled.
//
//	collector, err := factory.New(cfg.Metrics).
//	    UseLogger(logger).
//	    Build()
package factory
