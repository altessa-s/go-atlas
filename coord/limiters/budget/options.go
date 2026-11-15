// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package budget

//go:generate go tool optgen generate

import (
	"github.com/altessa-s/go-atlas/observability/metrics"
)

// options contains Limiter configuration.
type options struct {
	meter metrics.Meter `optgen:"notnil"`
}
