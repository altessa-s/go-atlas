// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package interceptors

// Matcher determines if an interceptor should be applied at runtime.
// Unlike static boolean conditions, Matcher is evaluated dynamically,
// allowing interceptors to be enabled/disabled based on runtime state
// such as feature flags, health checks, or external configuration.
//
// Example:
//
//	type featureFlagMatcher struct {
//	    flags *FeatureFlags
//	    flag  string
//	}
//
//	func (m *featureFlagMatcher) Match() bool {
//	    return m.flags.IsEnabled(m.flag)
//	}
type Matcher interface {
	// Match returns true if the interceptor should be applied.
	Match() bool
}

// MatchFunc adapts a function to the Matcher interface.
// This provides a convenient way to create Matchers from simple functions.
//
// Example:
//
//	matcher := interceptors.MatchFunc(func() bool {
//	    return featureFlags.IsEnabled("new-auth")
//	})
//	auth := interceptors.ServerMatchInterceptor(matcher, authInterceptor)
type MatchFunc func() bool

// Match implements the Matcher interface by calling the underlying function.
func (f MatchFunc) Match() bool { return f() }

// Interceptor is the minimal contract that every interceptor in a [Chain]
// must satisfy. The returned name is used for dependency-based topological
// ordering (see [depgraph]) and for diagnostic logging. Names must be
// unique within a single chain; duplicates cause a build-time error.
type Interceptor interface {
	Name() string
}
