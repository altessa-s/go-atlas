// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory maps the mongo block of the idempotency storage configuration
// to a MongoDB idempotency storage for factory/reliability/idempotency.
//
// The idempotency factory builds only the in-memory storage; register this one
// for the mongo type:
//
//	keeper, err := idempotencyfactory.New(cfg.Idempotency).
//		UseStorage(idempotencyconfig.StorageTypeMongo, mongofactory.Builder(db)).
//		Build()
package factory
