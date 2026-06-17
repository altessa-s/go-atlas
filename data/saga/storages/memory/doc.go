// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package memory provides an in-process, concurrency-safe [saga.Store] backed
// by a map. It is the reference implementation of the saga storage contract and
// is suited to single-node deployments and tests.
//
// State lives only in memory and is lost when the process exits, so it provides
// no cross-restart crash recovery — use a durable backend for that. Within a
// single process it fully supports the contract, including optimistic-concurrency
// updates ([Store.Update] is compare-and-swap on the instance version) and
// recovery queries ([Store.FetchRecoverable]).
//
// # Usage
//
//	store := memory.New()
//	orch := saga.New(store, def)
//
//	inst, err := orch.Start(ctx, id, data)
package memory
