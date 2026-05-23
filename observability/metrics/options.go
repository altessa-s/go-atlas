// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

package metrics

import (
	"github.com/altessa-s/go-atlas/observability/metrics/adapters"

	_ "github.com/altessa-s/go-atlas/core/runtime/appinfo"
)

// options contains configuration for the Collector.
type options struct {
	// serviceName is the global prefix for all metrics created by this collector.
	// It becomes the prefix for all metric names: {serviceName}_{subsystem}_{name}
	serviceName string `optgen:"default=appinfo.Name"`

	// adapter receives metric events.
	// For multiple backends, use adapters.NewMultiAdapter().
	adapter adapters.Adapter
}
