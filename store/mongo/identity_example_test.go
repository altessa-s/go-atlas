// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo_test

import (
	"context"

	"github.com/altessa-s/go-atlas/store/mongo"

	mongodriver "go.mongodb.org/mongo-driver/v2/mongo"
)

// ExampleWithDeduplicationIdentity keeps the documented snippet compiling:
// the identity comes from a request-scoped value.
func ExampleWithDeduplicationIdentity() {
	type tenantKey struct{}
	m, _ := mongo.New("app", mongo.WithReadDeduplication(), mongo.WithDeduplicationIdentity(func(ctx context.Context) string {
		tenant, _ := ctx.Value(tenantKey{}).(string)
		return tenant
	}))
	_ = m
}

// ExampleWithListCursorSubject keeps the documented snippet compiling.
func ExampleWithListCursorSubject() {
	type User struct{ Name string }
	type tenantKey struct{}
	var (
		ctx   = context.Background()
		coll  *mongodriver.Collection
		store mongo.Storage
	)

	tenant, _ := ctx.Value(tenantKey{}).(string)
	res, err := mongo.ListCursor[User](ctx, coll,
		mongo.WithListCursorStorage(store),
		mongo.WithListCursorSubject(tenant),
	)
	_, _ = res, err
}
