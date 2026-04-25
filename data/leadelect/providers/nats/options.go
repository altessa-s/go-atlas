// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate

import (
	"log/slog"
	"time"

	"github.com/altessa-s/go-atlas/observability/metrics"
)

// DefaultBucket is the default KeyValue bucket name.
const DefaultBucket = "leadelect"

// DefaultBucketKeysTTL is the default TTL for keys (10 seconds).
const DefaultBucketKeysTTL = time.Second * 10

// DefaultRenewRatio is the default lease renewal ratio (0.75).
const DefaultRenewRatio = 0.75

// options contains NATS provider configuration.
type options struct {
	logger     *slog.Logger
	bucket     string            `optgen:"default=DefaultBucket"`
	renewRatio float64           `optgen:"default=DefaultRenewRatio"`
	collector  metrics.Collector `optgen:"notnil"`
}
