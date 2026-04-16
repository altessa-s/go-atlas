// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides a fluent builder for creating audit auditors
// from configuration.
//
// [AuditorBuilder] uses a fluent API with deferred error accumulation:
// errors from any step are collected and returned at [AuditorBuilder.Build] time.
//
//	auditor, err := factory.New(cfg.Audit).
//	    UseLogger(logger).
//	    UseDispatcher(eng).
//	    Build()
package factory
