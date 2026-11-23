// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package broker

//go:generate go tool optgen generate

import (
	"log/slog"

	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/observability/tracing"
)

// DefaultSubjectLabelLimit caps the distinct subject label values of the
// publisher metrics; further subjects are counted under [OtherSubjectLabel].
const DefaultSubjectLabelLimit = 256

// options contains configuration fields for Broker that can be set via Option functions.
type options struct {
	// WithTracer enables consumer spans on subscribers created by the broker.
	tracer tracing.Tracer `optgen:"notnil"`
	// WithMessagingSystem sets the messaging.system attribute on consumer spans.
	messagingSystem string
	// WithConsumerName supplies the low-cardinality group name for consumer spans.
	consumerName string

	outbox           Outboxer `optgen:"notnil"`
	logger           *slog.Logger
	publishConverter PublishConverter
	meter            metrics.Meter `optgen:"notnil"`
	// subjectLabelLimit bounds publisher metric cardinality when the
	// application derives subjects from data (per tenant, per entity).
	subjectLabelLimit int `optgen:"default=DefaultSubjectLabelLimit"`
}
