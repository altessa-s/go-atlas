// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package prometheus

import (
	"sync"
)

// serverMetricsSingleton holds the singleton instance for HTTP server metrics.
// Initialization is protected by [sync.Once], making it safe for concurrent
// access from multiple goroutines. This ensures that Prometheus metrics are
// registered only once per process, preventing "already registered" errors
// when [New] is called multiple times.
var (
	serverMetricsOnce     sync.Once
	serverMetricsInstance *Middleware
)

// getOrCreateServerMetrics returns the singleton server metrics instance.
// On first call, it initializes and registers all Prometheus metrics.
// On subsequent calls, it returns the cached instance.
//
// Note: This function uses the options from the first call to configure metrics.
// If different options are needed, use the registerer option to register
// metrics with a custom registry.
func getOrCreateServerMetrics(opts *options) *Middleware {
	serverMetricsOnce.Do(func() {
		serverMetricsInstance = &Middleware{
			opts: opts,
		}
		serverMetricsInstance.initializeMetrics()
		serverMetricsInstance.registerMetrics()
	})

	return serverMetricsInstance
}
