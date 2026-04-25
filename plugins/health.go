// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package plugins

import (
	"context"

	"github.com/altessa-s/go-atlas/observability/health"
)

var _ health.Checker = (*Manager)(nil)

// CheckHealth implements health.Checker.
//
// The status mapping is:
//
//   - [health.StatusNotServing] when the manager has been closed, when the
//     sandbox setup failed (in which case the manager refuses every Load so
//     the registry stays empty), or when every loaded plugin is in
//     [StateFailed].
//   - [health.StatusDegraded] when at least one plugin is in [StateFailed]
//     but at least one other plugin is still operational. Plugin failures
//     are isolated — the host and its working plugins continue to serve
//     traffic, so "degraded" is a more honest signal than "not serving".
//     Also when the most recent watcher-driven [Manager.Reload] failed
//     (visible via [Manager.LastWatcherReloadErr]), even if every loaded
//     plugin is otherwise healthy: a stuck hot-reload pipeline is a real
//     operational concern but does not warrant tearing down the replica.
//   - [health.StatusServing] when no plugin is in [StateFailed] and the
//     watcher (if running) has no pending error.
func (m *Manager) CheckHealth(_ context.Context) health.ServingStatus {
	if m.closed.Load() {
		return health.StatusNotServing
	}
	if m.sandboxFailure() != nil {
		return health.StatusNotServing
	}

	m.mu.RLock()
	var failed, healthy int
	for _, p := range m.plugins {
		if p.State() == StateFailed {
			failed++
			continue
		}
		healthy++
	}
	m.mu.RUnlock()

	// A pending watcher reload error is not fatal but downgrades to
	// StatusDegraded. Read after the registry walk so the snapshot
	// reflects the most recent watcher attempt against the most
	// recent plugin set.
	watcherReloadFailed := m.LastWatcherReloadErr() != nil

	switch {
	case failed == 0 && !watcherReloadFailed:
		return health.StatusServing
	case failed > 0 && healthy == 0:
		return health.StatusNotServing
	default:
		// failed > 0 && healthy > 0    → degraded (mixed state)
		// failed == 0 && watcherFailed → degraded (hot-reload broken)
		return health.StatusDegraded
	}
}
