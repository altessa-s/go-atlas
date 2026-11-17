// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory maps the mongo block of the retry queue storage
// configuration to a MongoDB retry queue store for
// factory/reliability/retryqueue.
//
// The retry queue factory builds only the memory and sqldb stores; register
// this one for the mongo type:
//
//	queue, err := retryqueuefactory.New[Valuation](&cfg).
//		UseStorage(retryqueueconfig.StorageTypeMongo, mongofactory.Builder[Valuation](db)).
//		Build(ctx)
package factory
