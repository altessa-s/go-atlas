// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides a fluent builder for creating metrics collectors
// from configuration.
//
// Returns [metrics.Noop] when config is nil or metrics are disabled.
//
// The builder knows no concrete adapter: register one per metrics type with
// [CollectorBuilder.UseAdapter]. A configured type without a registered
// builder fails at Build with an error naming the module to add.
//
//	collector, err := factory.New(cfg.Metrics).
//	    UseAdapter(observabilityconfig.MetricsTypePrometheus, promfactory.Builder()).
//	    UseLogger(logger).
//	    Build()
package factory
