// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package memory provides an in-process, concurrency-safe
// [retryqueue.Storage] backed by a map. It is the reference implementation of
// the retry queue contract and suits tests and single-process deployments.
//
// Items live only in memory and are lost when the process exits. Every method
// runs under one mutex, so each transition is atomic and the exactly-once
// reports of [Store.Mark], [Store.Expire] and [Store.Flag] hold across
// goroutines. Resolved items are kept until deleted: there is no retention.
//
// # Usage
//
//	queue := memory.New[Valuation]()
//	item, err := queue.Mark(ctx, retryqueue.Item[Valuation]{Key: key, Since: now, NextAttemptAt: now})
package memory
