// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cache

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate

import (
	"time"

	"github.com/altessa-s/go-atlas/core/encoding/serializer"
	"github.com/altessa-s/go-atlas/observability/metrics"
)

// DefaultTTL is the default time-to-live for cache items (1 hour).
const DefaultTTL = 1 * time.Hour

// options contains Cache configuration.
type options struct {
	ttl         time.Duration `optgen:"default=DefaultTTL"`
	negativeTtl time.Duration // zero = disabled
	serializer  serializer.Serializer
	collector   metrics.Collector `optgen:"notnil"`
	name        string
}
