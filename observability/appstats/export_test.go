// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package appstats

// ResetMetricsCollection stops the background collection and forgets when the
// cached metrics were last refreshed, so that [IsCacheStale] reports true
// until the next collection tick, whatever earlier tests left behind.
func ResetMetricsCollection() {
	StopMetricsCollection()
	cachedMetrics.CPUTimestamp.Store(0)
	cachedMetrics.MemoryTimestamp.Store(0)
	cachedMetrics.NetworkTimestamp.Store(0)
}
