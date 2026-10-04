// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package pool

import "errors"

var (
	// ErrAlreadyStarted rejects a second cleanup lifecycle.
	ErrAlreadyStarted = errors.New("connection pool already started")
	// ErrFactoryRequired rejects a binding without a policy.
	ErrFactoryRequired = errors.New("connection factory required")
	// ErrConnectionPoolClosed is returned by [ConnectionPool.GetConnection] and
	// [ConnectionPool.Start] after the pool has been stopped.
	ErrConnectionPoolClosed = errors.New("connection pool is closed")
)
