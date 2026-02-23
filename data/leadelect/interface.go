// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package leadelect

import (
	"context"
)

// LeaderElector defines the interface for querying leader election status.
// Implementations must be safe for concurrent use.
type LeaderElector interface {
	// LeaderId returns the current leader's ID.
	LeaderId(ctx context.Context) (string, error)

	// IsLeader returns true if this instance is the leader.
	IsLeader() bool

	// NodeId returns this node's unique identifier.
	NodeId() string

	// IsRunning returns true if the election process is active.
	IsRunning() bool
}

// Callback is a function invoked on leadership changes.
// The context is canceled when the election stops or handler timeout expires.
type Callback func(ctx context.Context, p LeaderElector)
