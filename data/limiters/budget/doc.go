// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package budget implements a distributed budget limiter for outbound requests.
// It tracks the total number of requests per key within a configurable time period,
// rejecting requests that exceed the budget with [ErrBudgetExhausted].
//
// Unlike [github.com/altessa-s/go-atlas/data/limiters/tokenbucket], which performs
// inbound per-client rate limiting via the [github.com/altessa-s/go-atlas/data/limiters.Limiter]
// interface, budget enforces an aggregate outbound request cap per key. It does not
// implement the Limiter interface because it operates on explicit string keys rather
// than deriving identity from the request context.
//
// The limiter reuses storage backends from [github.com/altessa-s/go-atlas/data/limiters/storages]
// (memory, Redis, NATS) for distributed counter state, so budget
// enforcement stays consistent across service replicas.
//
// # Usage
//
//	limiter, err := budget.New(&budget.Settings{Limit: 10000, Period: 24 * time.Hour}, storage)
//	if err != nil {
//	    // handle error
//	}
//
//	if err := limiter.Allow(ctx, "service:recognizer"); err != nil {
//	    // budget exhausted or storage error
//	}
package budget
