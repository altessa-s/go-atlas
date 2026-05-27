// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package leadelect

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

import (
	"time"

	"github.com/altessa-s/go-atlas/observability/metrics"
)

// DefaultHandlerTimeout is the default timeout for callback execution (3 seconds).
const DefaultHandlerTimeout = 3 * time.Second

// DefaultTTL is the default lease duration before leadership expires.
// Matches the YAML default of [config.LeaderElector.Ttl].
const DefaultTTL = 10 * time.Second

// options contains Leader configuration.
type options struct {
	ttl            time.Duration     `optgen:"default=DefaultTTL"`
	handlerTimeout time.Duration     `optgen:"default=DefaultHandlerTimeout"`
	collector      metrics.Collector `optgen:"notnil"`
}
