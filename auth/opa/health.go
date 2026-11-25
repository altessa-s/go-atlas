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
	if m.snapshot.Load() == nil {
		return health.StatusNotServing
	}
	return health.StatusServing
}

func (m *Manager) registerHealth(reg health.Registrar) {
	var unregister func()
	if scoped, ok := reg.(health.ScopedReporter); ok {
		_, unregister = scoped.RegisterScopedService("opa", m)
	} else {
		checker := health.NewDetachableChecker(m)
		reg.RegisterService("opa", checker)
		unregister = checker.Detach
	}
	// A registrar may expose the manager while registration is in progress.
	m.mu.Lock()
	if m.closed.Load() {
		m.mu.Unlock()
		unregister()
		return
	}
	m.unregisterHealth = unregister
	m.mu.Unlock()
}
