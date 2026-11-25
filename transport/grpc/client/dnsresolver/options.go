// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package dnsresolver

//go:generate go tool optgen generate --type=options

import (
	"log/slog"
	"time"

	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/transport/grpc/client/internal/dnshook"
)

// Scheme is the target scheme the builder serves. [grpc.NewClient] gives a
// target without a registered scheme this scheme as well.
const Scheme = "dns"

// Default option values.
const (
	// DefaultRefreshInterval is how often a resolver re-resolves its host while
	// lookups succeed. A pod added behind a headless service is seen within
	// this interval.
	DefaultRefreshInterval = 30 * time.Second

	// DefaultRefreshJitter is the largest fraction by which a refresh interval
	// is shortened at random, so that clients started together, by one rollout
	// for example, do not query the DNS server in lockstep.
	DefaultRefreshJitter = 0.1

	// MaxRefreshJitter bounds the refresh jitter, so that a refresh never
	// follows the previous one sooner than half the refresh interval.
	MaxRefreshJitter = 0.5

	// DefaultMinInterval is the shortest pause between the end of a lookup and
	// a lookup that ResolveNow requests. gRPC asks for re-resolution once per
	// failing subchannel, so one server failure arrives as a burst; the pause
	// folds the burst into one lookup and stays below the backoff cap, so that
	// ResolveNow still speeds up a retry while lookups fail.
	DefaultMinInterval = 2 * time.Second

	// DefaultLookupTimeout bounds one lookup. It covers the default per-query
	// timeout of resolv.conf (5s) with one retry.
	DefaultLookupTimeout = 10 * time.Second

	// DefaultBackoffBaseDelay is the delay after the first failed lookup.
	DefaultBackoffBaseDelay = time.Second

	// DefaultBackoffMultiplier is the factor the delay grows by after each
	// further failed lookup.
	DefaultBackoffMultiplier = 1.6

	// DefaultBackoffJitter is the fraction by which each delay is randomized,
	// so the clients of one server do not retry in lockstep.
	DefaultBackoffJitter = 0.2

	// DefaultBackoffMaxDelay caps the delay between failed lookups, and so the
	// time a client needs to see addresses that return after an outage.
	DefaultBackoffMaxDelay = 5 * time.Second
)

// options holds the configuration of a resolver builder. It is modified
// through the generated [Option] functions.
type options struct {
	// WithRefreshInterval sets how often a resolver re-resolves its host while lookups succeed.
	refreshInterval time.Duration `optgen:"default=DefaultRefreshInterval"`
	// WithRefreshJitter sets the largest fraction by which a refresh interval is shortened at random; 0 disables jitter.
	// A value above MaxRefreshJitter (0.5) is lowered to it.
	refreshJitter float64 `optcheck:"min=0" optgen:"default=DefaultRefreshJitter,onfail=skip"`
	// WithMinInterval sets the shortest pause between the end of a lookup and a lookup that ResolveNow requests.
	// A value above the refresh interval is lowered to it.
	minInterval time.Duration `optgen:"default=DefaultMinInterval"`
	// WithLookupTimeout bounds one lookup.
	lookupTimeout time.Duration `optgen:"default=DefaultLookupTimeout"`
	// WithBackoffBaseDelay sets the delay after the first failed lookup.
	backoffBaseDelay time.Duration `optgen:"default=DefaultBackoffBaseDelay"`
	// WithBackoffMultiplier sets the factor the delay grows by after each further failed lookup.
	// A value below 1 is raised to 1.
	backoffMultiplier float64 `optgen:"default=DefaultBackoffMultiplier" optval:"positive"`
	// WithBackoffJitter sets the fraction by which each delay is randomized; 0 disables jitter.
	// A value above 1 is lowered to 1.
	backoffJitter float64 `optcheck:"min=0" optgen:"default=DefaultBackoffJitter,onfail=skip"`
	// WithBackoffMaxDelay caps the delay between failed lookups. Jitter never raises a delay above it.
	backoffMaxDelay time.Duration `optgen:"default=DefaultBackoffMaxDelay"`
	// WithLogger sets the logger for resolution state transitions.
	logger *slog.Logger `optgen:"default=slog.New(slog.DiscardHandler)"`
	// WithMeter enables the lookup and address count metrics.
	meter metrics.Meter
	// lookup replaces the host lookup of a [net.Resolver]. Set by withLookup,
	// which tests reach through the internal dnshook package.
	lookup dnshook.LookupFunc `opt:"-"`
}

// normalize brings option values into the ranges the resolver relies on.
func (o *options) normalize() {
	o.minInterval = min(o.minInterval, o.refreshInterval)
	o.refreshJitter = min(o.refreshJitter, MaxRefreshJitter)
	o.backoffMultiplier = max(o.backoffMultiplier, 1)
	o.backoffJitter = min(o.backoffJitter, 1)
}
