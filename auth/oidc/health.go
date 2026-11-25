// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"context"

	"github.com/altessa-s/go-atlas/observability/health"
)

var _ health.Checker = (*Provider)(nil)

// CheckHealth implements health.Checker.
func (p *Provider) CheckHealth(_ context.Context) health.ServingStatus {
	if p.backgroundCtx != nil && p.backgroundCtx.Err() != nil {
		return health.StatusNotServing
	}
	if p.discovery.info == nil || !p.discovery.info.IsValid() {
		return health.StatusNotServing
	}
	if p.keys.jwks == nil {
		return health.StatusNotServing
	}
	return health.StatusServing
}

func (p *Provider) registerHealth(reg health.Registrar) {
	var unregister func()
	if scoped, ok := reg.(health.ScopedReporter); ok {
		_, unregister = scoped.RegisterScopedService("oidc", p)
	} else {
		checker := health.NewDetachableChecker(p)
		reg.RegisterService("oidc", checker)
		unregister = checker.Detach
	}
	// A registrar can expose the provider before this call returns.
	p.healthMu.Lock()
	if p.backgroundCtx.Err() != nil {
		p.healthMu.Unlock()
		unregister()
		return
	}
	p.unregisterHealth = unregister
	p.healthMu.Unlock()
}
