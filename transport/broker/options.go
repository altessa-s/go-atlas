// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package broker

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate

import (
	"log/slog"

	"github.com/altessa-s/go-atlas/observability/metrics"
)

// options contains configuration fields for Broker that can be set via Option functions.
type options struct {
	outbox           Outboxer `optgen:"notnil"`
	logger           *slog.Logger
	publishConverter PublishConverter
	collector        metrics.Collector `optgen:"notnil"`
}
