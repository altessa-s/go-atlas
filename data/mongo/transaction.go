// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/mongo"
)

// InTransaction reports whether ctx carries a session with a running transaction (started and neither committed nor aborted), as the
// context a [Mongo.WithTransaction] handler receives does. A session without a transaction (causal consistency only) is not one.
//
// Use it where a transaction changes what an operation may do: a write error aborts the whole transaction, so a duplicate-key retry
// must not run inside one (only a retry of the transaction can recover), and a cache must not serve or store a transaction's reads.
func InTransaction(ctx context.Context) bool {
	sess := mongo.SessionFromContext(ctx)
	return sess != nil && sess.TransactionRunning()
}

// joinableTransaction reports whether ctx carries a running transaction of client, which [Mongo.WithTransaction] joins.
func joinableTransaction(ctx context.Context, client *mongo.Client) bool {
	sess := mongo.SessionFromContext(ctx)
	return client != nil && sess != nil && sess.TransactionRunning() && sess.Client() == client
}
