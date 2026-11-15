// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory maps the nats block of the limiter storage configuration
// to a NATS JetStream limiter storage for factory/coord/limiters/budget and
// factory/coord/limiters/tokenbucket.
//
// The limiter factories build only the in-memory storage; register this one
// for the nats type:
//
//	limiter, err := budgetfactory.New(cfg.BudgetLimiter).
//		UseStorage(storageconfig.BackendTypeNATS, natsfactory.Builder(js)).
//		Build(ctx)
package factory
