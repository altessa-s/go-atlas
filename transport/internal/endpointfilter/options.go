// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package endpointfilter

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate --type=options

import (
	"regexp"
)

const (
	// DefaultIgnoreCacheSize is the default LRU cache capacity for
	// positive filter matches (1024 distinct paths).
	DefaultIgnoreCacheSize = 1024

	// MaxRecommendedCacheSize is an advisory upper bound. Caches larger
	// than this consume significant memory with diminishing hit-rate
	// improvements for typical workloads.
	MaxRecommendedCacheSize = 10000
)

// options holds [Checker] configuration populated by functional [Option]
// values.
type options struct {
	cacheSize      int `optgen:"default=DefaultIgnoreCacheSize"`
	ignorePatterns []*regexp.Regexp
}
