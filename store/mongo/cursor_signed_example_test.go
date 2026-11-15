// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo_test

import (
	"context"

	"github.com/altessa-s/go-aip/pagination/pagetoken"

	"github.com/altessa-s/go-atlas/store/mongo"

	mongodriver "go.mongodb.org/mongo-driver/v2/mongo"
)

// ExampleWithListCursorSigner keeps the documented snippet compiling.
func ExampleWithListCursorSigner() {
	type User struct{ Name string }
	type tenantKey struct{}
	var (
		ctx        = context.Background()
		coll       *mongodriver.Collection
		signingKey []byte
	)

	codec, err := pagetoken.New(signingKey)
	if err != nil {
		return
	}
	tenant, _ := ctx.Value(tenantKey{}).(string)
	res, err := mongo.ListCursor[User](ctx, coll,
		mongo.WithListCursorSigner(codec),
		mongo.WithListCursorSubject(tenant),
	)
	_, _ = res, err
}
