// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package noop provides no-operation distributed lock for testing without external dependencies.
// All lock operations succeed immediately. Ideal for unit tests and development environments.
//
// Example:
//
//	dl := dlock.NewWithNoop()
//	defer dl.Close()
//	lock, _ := dl.Lock(ctx, "test-resource")
//	defer lock.Release(ctx)
//	// Or use with critical section:
//	dl.Synchronize(ctx, "resource", func(ctx context.Context) error {
//		return processTestData()
//	})
package noop
