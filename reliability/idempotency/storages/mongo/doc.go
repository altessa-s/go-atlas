// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package mongo provides MongoDB storage for idempotency keys. Each key is a
// document locked with one atomic upsert and compared-and-set on its stored
// bytes; a TTL index expires old keys.
//
// # Transactions
//
// Every operation runs on the context it is given. Called inside
// mongo.Session.WithTransaction, the lock and the stored result commit or
// abort together with the caller's own writes: an aborted transaction leaves
// no lock behind, and a duplicate racing in another transaction hits a write
// conflict, retries, and then sees the committed key.
//
// Example:
//
//	storage, err := mongo.New(db, mongo.WithTTL(24*time.Hour))
//	keeper := idempotency.New(storage, idempotency.WithMaxLockDuration(time.Minute))
//	_, err = sess.WithTransaction(ctx, func(ctx context.Context) (any, error) {
//	    ok, held, err := keeper.AttemptLock(ctx, key)
//	    if err != nil || !ok {
//	        return nil, err
//	    }
//	    // business writes on ctx
//	    return nil, keeper.Complete(ctx, key, result, held)
//	})
package mongo
