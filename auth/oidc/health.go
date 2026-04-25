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
	if p.discoveryInfo == nil || !p.discoveryInfo.IsValid() {
		return health.StatusNotServing
	}
	if p.jwks == nil {
		return health.StatusNotServing
	}
	return health.StatusServing
}
