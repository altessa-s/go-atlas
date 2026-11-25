// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory maps the nats block of the idempotency storage configuration
// to a NATS JetStream KV idempotency storage for factory/reliability/idempotency.
//
// The idempotency factory builds only the in-memory storage; register this one
// for the nats type:
//
//	keeper, err := idempotencyfactory.New(cfg.Idempotency).
//		UseStorage(idempotencyconfig.StorageTypeNATS, natsfactory.Builder(js)).
//		Build(ctx)
package factory
