// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package uowtest provides a scriptable in-memory transaction for testing code
// built on [github.com/altessa-s/go-atlas/messaging/eventbus/uow].
//
// # Fake
//
// [Fake] is a uow.Committer and [Transaction] is its uow.Detector. Its fields script a driver: callback retries after a transient
// abort (Attempts), commit-only retries (CommitRetries), a definite failure
// (Fail, Abort), a lost commit reply with the writes kept (Unknown), and
// recovery of a callback panic into an error (RecoverPanics), as
// store/mongo.Mongo.WithTransaction does. Stores listed in Isolate implement
// [Snapshotter] and are rolled back after every aborted attempt, so tests
// observe real all-or-nothing state.
//
// # Usage
//
//	f := &uowtest.Fake{Attempts: 2, Isolate: []uowtest.Snapshotter{store}}
//	runner := uow.New(f, nil, uow.WithDetector(uowtest.Transaction))
//	err := runner.Run(ctx, func(txCtx context.Context) error {
//		return store.Save(txCtx, doc)
//	})
package uowtest
