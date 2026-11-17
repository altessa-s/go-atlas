// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package lockconfig defines distributed lock and leader election schemas.
//
// Schemas carry yaml and default tags read by
// github.com/altessa-s/go-nomos/loader, expose Default* constructors
// where defaults exist, and implement Validate; component factories map them
// to generated options.
//
// Key types: [DistributionLockProvider], [DistributionLockMongo], [DistributionLockSQL],
// [DistributionLockNATS], [DistributionLock], [LeaderElectorProvider],
// [LeaderElector].
//
// # Purging
//
// The sqldb block exposes purgeInterval (default "1m", "0s" disables the background loop) and
// purgeBatchSize (default 1000). Manual PurgeExpired remains available with a zero interval. Purging
// runs only when retention is enabled; idempotency keys always have their own expiry.
package lockconfig
