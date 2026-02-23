// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

import (
	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

// Interned header names for memory efficiency.
var (
	InternedHeaderIdempotencyKey = corestrings.InternString(DefaultIdempotencyKeyHeader)
)
