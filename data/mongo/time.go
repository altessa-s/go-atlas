// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import "time"

// Now returns the current time as MongoDB stores it: UTC, truncated to the millisecond, so a value written and read back compares
// equal to the one the caller kept.
func Now() time.Time { return StoredTime(time.Now()) }

// StoredTime returns t as MongoDB stores it: in UTC, truncated (floored) to the millisecond, without a monotonic clock reading.
func StoredTime(t time.Time) time.Time { return t.UTC().Truncate(time.Millisecond) }
