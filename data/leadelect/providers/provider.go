// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package providers

import (
	"context"
	"time"
)

// Provider defines the interface for leader election backends.
// Implementations must be safe for concurrent use.
type Provider interface {
	// LeaderId returns the current leader's ID.
	LeaderId(ctx context.Context) (string, error)

	// IsLeader returns true if this instance is the leader.
	IsLeader() bool

	// Fence returns the fencing token of the leadership term this node currently
	// holds, or 0 when it is not a fresh leader. The token is monotonically
	// non-decreasing across terms: every acquisition or renewal observes a token
	// greater than or equal to the previous holder's, and a new holder always
	// observes a strictly greater token than any prior term. Downstream storage
	// can record the highest token it has accepted and reject writes carrying a
	// lower one, fencing out a stale or partitioned zombie leader whose IsLeader
	// has not yet self-demoted. It is gated by IsLeader, so a lease that has
	// expired by the freshness bound reports 0.
	Fence() uint64

	// NodeId returns this node's unique identifier.
	NodeId() string

	// IsRunning returns true if the election is active.
	IsRunning() bool

	// Start begins the leader election process.
	Start(ctx context.Context, cfg Config) error

	// Stop terminates the election and resigns leadership.
	Stop(ctx context.Context) error
}

// Config holds provider configuration and notification channels.
type Config struct {
	// Key is the election group identifier.
	Key string

	// TTL is the lease duration.
	TTL time.Duration

	// NodeId is this node's unique identifier.
	NodeId string

	// LostCh receives notifications when leadership is lost.
	LostCh chan<- struct{}

	// BecameCh receives notifications when leadership is acquired.
	BecameCh chan<- struct{}

	// StopCh receives notifications when the provider stops.
	StopCh chan<- struct{}
}
