// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nilcheck

import "fmt"

// Checker provides fluent nil validation with error accumulation for verifying
// that required dependencies are non-nil. It is designed for constructor and
// factory functions that need to validate multiple inputs before proceeding.
//
// A Checker is not safe for concurrent use. Create a new instance with [NewChecker]
// for each validation sequence. After validation, inspect results with [Checker.Error],
// [Checker.Errors], or [Checker.HasErrors].
//
// Example:
//
//	checker := nilcheck.NewChecker("MyFactory")
//	if err := checker.Check(db, "database").Check(cache, "cache").Error(); err != nil {
//	    return nil, err
//	}
type Checker struct {
	contextName string
	errors      []error
}

// NewChecker creates a new [Checker] with the given context name.
// The contextName appears as a prefix in all error messages produced by
// [Checker.Check], formatted as "<contextName>: <depName> is required".
func NewChecker(contextName string) *Checker {
	return &Checker{contextName: contextName}
}

// Check validates that value is not nil using [IsNil] and records an error if it is.
// The depName identifies the dependency in the error message. Returns the Checker
// to allow fluent method chaining. Multiple calls accumulate independent errors.
func (c *Checker) Check(value any, depName string) *Checker {
	if IsNil(value) {
		c.errors = append(c.errors, fmt.Errorf("%s: %s is required", c.contextName, depName))
	}
	return c
}

// Error returns the first accumulated error, or nil if all checks passed.
// When only the first failure matters, prefer this over [Checker.Errors].
func (c *Checker) Error() error {
	if len(c.errors) == 0 {
		return nil
	}
	return c.errors[0]
}

// Errors returns all accumulated validation errors from [Checker.Check] calls.
// Returns nil if no errors were recorded. The returned slice should not be modified.
func (c *Checker) Errors() []error {
	return c.errors
}

// HasErrors reports whether any [Checker.Check] calls recorded a nil value.
func (c *Checker) HasErrors() bool {
	return len(c.errors) > 0
}

// Reset clears all accumulated errors, allowing the [Checker] to be reused for
// a new validation sequence. Returns the Checker for method chaining.
func (c *Checker) Reset() *Checker {
	c.errors = nil
	return c
}
