// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package principalowner

import (
	"context"

	"github.com/altessa-s/go-atlas/auth/principal"

	corekeys "github.com/altessa-s/go-atlas-core/encoding/keys"
	dataidempotency "github.com/altessa-s/go-atlas/reliability/idempotency"
)

var _ dataidempotency.OwnerFunc = PrincipalOwner

// PrincipalOwner labels an idempotency record with the verified principal's
// tenant and subject from ctx. A missing principal or subject has no owner.
func PrincipalOwner(ctx context.Context) string {
	p, ok := principal.FromContext(ctx)
	if !ok || p.Subject == "" {
		return ""
	}
	return corekeys.Compose(p.Tenant, p.Subject)
}
