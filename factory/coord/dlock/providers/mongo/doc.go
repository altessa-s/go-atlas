// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory maps the mongo block of the distribution lock
// configuration to a MongoDB lock provider for factory/coord/dlock.
//
// The dlock factory builds only the sqldb provider; register this one for
// the mongo type:
//
//	dl, err := dlockfactory.New(cfg.DistributionLock).
//		UseProvider(lockconfig.DistributionLockProviderMongo, mongofactory.Builder(db)).
//		Build(ctx)
package factory
