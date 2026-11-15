// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natsprovider

//go:generate go tool optgen generate

import (
	"time"

	"github.com/altessa-s/go-atlas/observability/metrics"
)

// DefaultDrainTimeout is the default grace period Unsubscribe gives the
// handlers of already buffered messages before canceling their context.
const DefaultDrainTimeout = 30 * time.Second

// options configures the [NATS] provider.
type options struct {
	// allowedSubjects ([WithAllowedSubjects]) restricts Publish and Subscribe
	// to the listed subject patterns. NATS wildcards are supported: "*"
	// matches a single token, ">" matches one or more trailing tokens. When
	// the allowlist is empty (default), all subjects are permitted.
	allowedSubjects []string `optval:"notrim"`
	// meter ([WithMeter]) is the metrics meter for NATS subscriber
	// instrumentation.
	meter metrics.Meter
	// drainTimeout ([WithDrainTimeout]) bounds how long a subscriber's
	// Unsubscribe waits for the handlers of already buffered messages before
	// it cancels their context. A non-positive value keeps the default.
	drainTimeout time.Duration `optgen:"default=DefaultDrainTimeout"`
}
