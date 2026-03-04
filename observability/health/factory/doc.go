// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides a fluent builder for creating health coordinators
// from configuration.
//
// [CoordinatorBuilder] uses a fluent API with deferred error accumulation:
// errors from any step are collected and returned at [CoordinatorBuilder.Build] time.
//
//	coordinator, err := factory.New(cfg.Health).
//	    UseLogger(logger).
//	    Build()
package factory
