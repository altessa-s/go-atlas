// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides a factory for creating [tracing.Tracer] instances
// from [config.Tracing] configuration.
//
// The factory handles:
//   - Creating [tracing.Tracer] from [config.Tracing] configuration
//   - Selecting and configuring appropriate adapters (OTLP, Console)
//   - Setting up samplers based on configuration
//   - Returns [tracing.Noop] when tracing is disabled
//
// # Basic Usage
//
//	f := factory.New(factory.WithLogger(logger))
//	provider, err := f.CreateFromConfig(&cfg.Observability.Tracing)
//	if err != nil {
//	    return err
//	}
//	defer provider.Shutdown(ctx)
//
// # Configuration-based Creation
//
// The factory reads from config.Tracing to determine:
//
//   - Whether tracing is enabled
//
//   - Which exporter type to use (OTLP, Console, Noop)
//
//   - Service name and version for spans
//
//   - Sampling configuration
//
//     observability:
//     tracing:
//     enable: true
//     type: otlp
//     serviceName: myapp
//     sampler:
//     type: parent_based
//     ratio: 0.1
//     otlp:
//     endpoint: localhost:4317
package factory
