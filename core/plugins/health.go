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
// The manager is considered unhealthy when it has been closed, when any
// loaded plugin is in [StateFailed], or when the sandbox setup failed —
// in the last case the manager refuses every Load so the registry stays
// empty and the per-plugin loop alone would falsely report "serving".
func (m *Manager) CheckHealth(_ context.Context) health.ServingStatus {
	if m.closed.Load() {
		return health.StatusNotServing
	}
	if m.sandboxFailure() != nil {
		return health.StatusNotServing
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, p := range m.plugins {
		if p.State() == StateFailed {
			return health.StatusNotServing
		}
	}

	return health.StatusServing
}
