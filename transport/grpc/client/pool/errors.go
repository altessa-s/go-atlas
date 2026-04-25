// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package pool

import "errors"

var (
	// ErrConnectionPoolClosed is returned by [ConnectionPool.GetConnection] and
	// [ConnectionPool.Start] after the pool has been stopped.
	ErrConnectionPoolClosed = errors.New("connection pool is closed")
)
