// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides a fluent builder for creating OPA managers
// from configuration. The startup context controls initial loading. Factory-started
// polling runs until the manager is stopped or closed.
//
// [ManagerBuilder] uses a fluent API with deferred error accumulation:
// errors from any step are collected and returned at [ManagerBuilder.Build] time.
//
//	manager, err := factory.New(cfg.OPA).
//	    UseLogger(logger).
//	    UseScheduler(scheduler).
//	    UseHealthCoordinator(hc).
//	    Build(ctx)
//
// Filesystem and S3 sources require a release manifest. Scheduled updates
// without bundle watching require a task registrar.
package factory
