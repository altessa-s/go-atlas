// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

import (
	"github.com/altessa-s/go-atlas-core/text/strings/intern"
)

// Interned header names for memory efficiency.
var (
	InternedHeaderIdempotencyKey = intern.String(DefaultIdempotencyKeyHeader)
)
