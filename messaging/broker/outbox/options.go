// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outbox

//go:generate go tool optgen generate --keep-empty-strings

import (
	"github.com/altessa-s/go-atlas/observability/tracing"

	dataoutbox "github.com/altessa-s/go-atlas/reliability/outbox"
)

type options struct {
	// WithTracer enables producer spans for sampled events.
	tracer tracing.Tracer
	// WithMessagingSystem sets the messaging.system attribute on producer spans.
	messagingSystem string `optval:"notrim"`
	// WithOutboxOptions appends options for the underlying transactional outbox.
	outboxOptions []dataoutbox.Option `optgen:"append"`
}
