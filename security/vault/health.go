// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package vault

import (
	"context"

	"github.com/altessa-s/go-atlas/observability/health"
)

var _ health.Checker = (*Vault)(nil)

// CheckHealth implements health.Checker.
func (v *Vault) CheckHealth(ctx context.Context) health.ServingStatus {
	v.mu.Lock()
	running := v.authCancel != nil
	v.mu.Unlock()

	if !running {
		return health.StatusNotServing
	}

	resp, err := v.opts.vaultClient.Sys().HealthWithContext(ctx)
	if err != nil {
		return health.StatusNotServing
	}

	if resp.Sealed {
		return health.StatusNotServing
	}

	if !resp.Initialized {
		return health.StatusNotServing
	}

	return health.StatusServing
}
