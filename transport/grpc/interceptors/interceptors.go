// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package interceptors

import "github.com/altessa-s/go-atlas/transport/internal/base"

// Matcher determines if an interceptor should be applied at runtime.
// Unlike static boolean conditions, Matcher is evaluated dynamically,
// allowing interceptors to be enabled/disabled based on runtime state
// such as feature flags, health checks, or external configuration.
type Matcher = base.Matcher

// MatchFunc adapts a function to the Matcher interface.
// This provides a convenient way to create Matchers from simple functions.
type MatchFunc = base.MatchFunc

// Interceptor is the minimal contract that every interceptor in a [Chain]
// must satisfy. The returned name is used for dependency-based topological
// ordering (see [depgraph]) and for diagnostic logging. Names must be
// unique within a single chain; duplicates cause a build-time error.
type Interceptor interface {
	Name() string
}
