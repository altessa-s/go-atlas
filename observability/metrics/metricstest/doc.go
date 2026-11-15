// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package metricstest provides an in-memory metrics collector and value readers for tests that assert on emitted metrics.
//
// Example:
//
//	tc := metricstest.NewTestCollector()
//	v := metricstest.GetCounterValue(t, tc, "requests_total", "code", "200")
package metricstest
