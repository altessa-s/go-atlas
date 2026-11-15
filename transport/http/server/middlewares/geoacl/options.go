// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package geoacl

//go:generate go tool optgen generate --type=options

import (
	"log/slog"
	"regexp"

	"github.com/altessa-s/go-atlas/transport/policy/fallback"
)

// options holds configuration for the geographic access control middleware.
type options struct {
	logger *slog.Logger
	// ignorePaths and ignorePatterns have no defaults: the middleware exempts
	// nothing unless the caller configures it.
	ignorePaths      []string
	ignorePatterns   []*regexp.Regexp
	fallbackBehavior fallback.Behavior `optgen:"default=fallback.Deny"`
	// WithAllowPrivateAddresses lets a client with a loopback, private,
	// link-local or otherwise reserved address through without a geographic
	// check: such an address has no location, so an allow list or a deny-by-default
	// registry would turn away health probes and in-cluster callers. Off by
	// default. The client address is the one realip resolved; enable this only
	// when forwarded headers are accepted from trusted proxies alone.
	allowPrivateAddresses bool
}
