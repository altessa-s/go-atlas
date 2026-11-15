// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides a fluent builder for creating go-atlas-core
// runtime/dispatch engines from configuration. [EngineBuilder.WithMeter]
// installs the metrics observer of the observe subpackage.
//
// [EngineBuilder] uses a fluent API with deferred error accumulation:
// errors from any step are collected and returned at [EngineBuilder.Build] time.
//
//	eng, err := factory.New[*audit.Event](cfg.Dispatch).
//	    WithSink(audit.StorageSink{Storage: store}).
//	    WithCodec(audit.JSONCodec{}).
//	    WithLogger(logger).
//	    Build()
package factory
