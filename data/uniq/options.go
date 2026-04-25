// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package uniq

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate

import (
	"log/slog"

	"github.com/altessa-s/go-atlas/core/encoding/serializer"
	"github.com/altessa-s/go-atlas/observability/health"
	"github.com/altessa-s/go-atlas/observability/metrics"
)

// DefaultHealthServiceName is the name under which Uniq registers itself
// with the health coordinator when no override is supplied.
const DefaultHealthServiceName = "uniq"

// options contains Uniq configuration.
type options struct {
	serializer serializer.Serializer
	collector  metrics.Collector `optgen:"notnil"`
	logger     *slog.Logger
	// healthCoordinator registers Uniq with a health coordinator on
	// construction. Disabled when nil.
	healthCoordinator *health.Coordinator
	// healthServiceName customizes the service name used for health
	// registration. Useful when multiple Uniq instances share one
	// coordinator (e.g. "uniq-tokens", "uniq-emails").
	healthServiceName string `optgen:"default=DefaultHealthServiceName"`
}
