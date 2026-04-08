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
//   - [health.StatusServing] when no plugin is in [StateFailed].
func (m *Manager) CheckHealth(_ context.Context) health.ServingStatus {
	if m.closed.Load() {
		return health.StatusNotServing
	}
	if m.sandboxFailure() != nil {
		return health.StatusNotServing
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	var failed, healthy int
	for _, p := range m.plugins {
		if p.State() == StateFailed {
			failed++
			continue
		}
		healthy++
	}

	switch {
	case failed == 0:
		return health.StatusServing
	case healthy == 0:
		return health.StatusNotServing
	default:
		return health.StatusDegraded
	}
}
