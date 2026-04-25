// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides common utilities for factory pattern implementations.
//
// This package contains base types and helper functions that can be used by
// concrete factory implementations throughout the codebase to ensure consistency
// in error handling, logging, and validation.
//
// # Base Factory
//
// The Base struct provides common logger handling:
//
//	type MyFactory struct {
//	    factory.Base
//	    // additional fields...
//	}
//
//	func New(opts ...Option) *MyFactory {
//	    cfg := newOptions(opts...)
//	    return &MyFactory{
//	        Base: factory.NewBase(cfg.logger),
//	    }
//	}
//
// # Error Helpers
//
// Use the error helper functions for consistent error messages:
//
//	if dep == nil {
//	    return nil, factory.RequiredError("redis client", "Redis storage")
//	}
//
//	if err := doSomething(); err != nil {
//	    return nil, factory.WrapError(err, "creating provider")
//	}
//
// # Validation
//
// For validation utilities, see the core/types/validation package which provides:
//   - FactoryValidator for validating configs and dependencies
//   - RequireNotNil for simple nil checks
//   - NilChecker for fluent nil validation chains
package factory
