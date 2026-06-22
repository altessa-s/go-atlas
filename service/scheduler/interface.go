// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scheduler

// LeaderElector is the narrow leader-election contract consumed by the
// scheduler. Any type exposing a concurrency-safe IsLeader method satisfies
// it — for example, *leadelect.Leader from data/leadelect.
type LeaderElector interface {
	// IsLeader reports whether this node currently holds leadership.
	// Implementations must be safe for concurrent use.
	IsLeader() bool
}
