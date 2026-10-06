// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package mongo implements a [providers.Provider] for distributed locks on
// MongoDB, for deployments that coordinate replicas through MongoDB rather
// than NATS.
//
// # Model
//
// Each lock key is one document in a collection (default "dlocks"). A lock
// is taken by a single atomic upsert that succeeds only while the key has no
// unexpired lease; every lease decision compares against the MongoDB server
// clock ($$NOW), so replicas with skewed clocks agree. Each acquisition
// increments the document's fencing token, exposed as
// [providers.LockInfo.FencingToken]; releasing a lock ends its lease but keeps
// the document, so tokens never restart.
//
// While the lock's context lives, the lease is renewed every TTL × renew
// ratio (default 10 s × 1/3). Renewal and release match the holder and its
// fencing token, so a holder whose lease expired and was taken over can
// neither renew nor release the new holder's lease. Ending the context
// releases the lease.
//
// # Usage
//
//	locker, err := mongo.New(db, mongo.WithTTL(15*time.Second))
//	if err != nil {
//	    return err
//	}
//	lk, err := locker.Lock(ctx, "ticket:42")
//	if errors.Is(err, errs.ErrLockNotHeld) {
//	    // another replica holds it
//	}
//	defer lk.Release(context.Background())
package mongo
