// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides a fluent builder for creating tracer providers
// from configuration.
//
// Build returns a tracing.TracerProvider. A nil or disabled configuration
// yields a no-op provider with the same lifecycle methods.
//
// Console and noop adapters are built in; register any other backend with
// [TracerBuilder.UseAdapter]. A configured type without a registered builder
// fails at Build with an error naming the module to add.
//
//	provider, err := factory.New(cfg.Tracing).
//	    UseAdapter(observabilityconfig.TracingTypeOTLP, otlpfactory.Builder()).
//	    UseLogger(logger).
//	    Build(ctx)
package factory
