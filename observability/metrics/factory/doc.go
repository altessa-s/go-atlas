// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides configuration-based creation of [metrics.Collector]
// instances from [config.Metrics].
//
// Returns [metrics.Noop] when config is nil or metrics are disabled.
//
// # Example
//
//	f := factory.New(factory.WithLogger(logger))
//	collector, err := f.CreateFromConfig(cfg.Metrics)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer collector.Shutdown(ctx)
package factory
