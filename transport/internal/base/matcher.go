// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package base

// Matcher determines if a middleware or interceptor should be applied at
// runtime. Unlike static boolean conditions, Matcher is evaluated
// dynamically, allowing components to be enabled/disabled based on runtime
// state such as feature flags, health checks, or external configuration.
type Matcher interface {
	// Match returns true if the component should be applied.
	Match() bool
}

// MatchFunc adapts a function to the Matcher interface.
type MatchFunc func() bool

// Match implements the Matcher interface by calling the underlying function.
func (f MatchFunc) Match() bool { return f() }
