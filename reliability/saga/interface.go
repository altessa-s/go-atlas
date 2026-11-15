// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package saga

// LeaderElector is the narrow leader-election contract consumed by the
// orchestrator to gate the background recovery cycle so that, in a multi-node
// deployment, only the elected leader scans the store. When set via
// [WithLeaderElector], the recovery cycle is a no-op on nodes whose IsLeader
// reports false. Gating is an optimization, not a correctness requirement: the
// store's optimistic-concurrency check already makes concurrent recovery cycles
// safe; the gate only avoids redundant scans and writes. A *leadelect.Leader
// satisfies this interface.
type LeaderElector interface {
	// IsLeader reports whether this node should run the recovery cycle now.
	IsLeader() bool
}
