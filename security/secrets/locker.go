// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package secrets

import "context"

// Locker defines the interface for distributed locking mechanisms.
type Locker interface {
	// Synchronize acquires a distributed lock for the specified key and
	// executes the provided function while holding the lock, and then releases the lock.
	Synchronize(ctx context.Context, key string, fn func(ctx context.Context) error) error
}
