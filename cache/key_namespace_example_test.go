// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cache_test

import (
	"context"

	"github.com/altessa-s/go-atlas/cache"

	storages "github.com/altessa-s/go-atlas/cache/storages"
)

// ExampleWithKeyNamespace keeps the documented snippet compiling: keys are
// scoped to the tenant supplied in the request context.
func ExampleWithKeyNamespace() {
	type tenantKey struct{}
	var provider storages.Storage
	c := cache.New(provider, cache.WithKeyNamespace(func(ctx context.Context) string {
		tenant, _ := ctx.Value(tenantKey{}).(string)
		return tenant
	}))
	_ = c
}
