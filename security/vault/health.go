// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package vault

import (
	"context"
	"fmt"

	"github.com/altessa-s/go-atlas/observability/health"
)

var _ health.Checker = (*Vault)(nil)

// CheckHealth implements health.Checker. The sys/health probe is
// unauthenticated and says nothing about the token, so it is consulted only
// for a client that has working authentication:
//
//   - with an auth method: renewal was started and not stopped, its run
//     logged in, and it has not ended with an error ([Vault.RenewalErr]). A
//     run that ended because the token never expires still counts;
//   - without an auth method: the client carries a token the caller set.
//
// A closed client never serves.
func (v *Vault) CheckHealth(ctx context.Context) health.ServingStatus {
	v.mu.Lock()
	closed := v.closed
	running := v.authCancel != nil
	authenticated := v.authenticated
	renewalErr := v.renewalErr
	v.mu.Unlock()

	if renewalErr != nil {
		// The raw error may carry request URLs; log its type only.
		v.logger.WarnContext(ctx, "vault health: token renewal ended unexpectedly, the client is not authenticated",
			"error_type", fmt.Sprintf("%T", renewalErr))
		return health.StatusNotServing
	}
	if v.opts.authMethod == nil {
		authenticated = v.opts.vaultClient.Token() != ""
	} else {
		authenticated = running && authenticated
	}
	if closed || !authenticated {
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
