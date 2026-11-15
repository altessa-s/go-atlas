// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency_test

import (
	"context"
	"time"

	"github.com/altessa-s/go-atlas/reliability/idempotency"
	"github.com/altessa-s/go-atlas/reliability/idempotency/storages"
)

// ExampleWithKeyNamespace keeps the documented snippet compiling: keys are
// scoped to the tenant supplied in the request context.
func ExampleWithKeyNamespace() {
	type tenantKey struct{}
	var storage storages.Storage
	k := idempotency.New(storage,
		idempotency.WithMaxLockDuration(2*time.Minute),
		idempotency.WithKeyNamespace(func(ctx context.Context) string {
			tenant, _ := ctx.Value(tenantKey{}).(string)
			return tenant // "" selects the empty namespace
		}),
	)
	_ = k
}
