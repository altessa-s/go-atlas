// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package opa

import (
	"context"

	"github.com/altessa-s/go-atlas/observability/health"
)

var _ health.Checker = (*Manager)(nil)

// CheckHealth implements health.Checker.
func (m *Manager) CheckHealth(_ context.Context) health.ServingStatus {
	if m.closed.Load() {
		return health.StatusNotServing
	}
	if v := m.lastError.Load(); v != nil && *v != nil {
		return health.StatusNotServing
	}
	if m.preparedEval.Load() == nil {
		return health.StatusNotServing
	}
	return health.StatusServing
}
