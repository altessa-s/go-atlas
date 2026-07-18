// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package probfilter

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

import (
	"github.com/altessa-s/go-atlas/observability/metrics"
)

// options contains Manager configuration.
type options struct {
	collector metrics.Collector `optgen:"notnil"`
}
