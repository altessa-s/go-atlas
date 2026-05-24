// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package memory provides an in-memory metrics adapter implementation
// for testing. Metric values are accumulated in goroutine-safe maps
// and exposed via lookup methods.
//
// # Usage
//
//	adapter := memory.NewAdapter()
//	collector := metrics.NewCollector(adapter, "test")
//
//	// Record metrics
//	collector.Counter("requests", "Total requests").
//	    WithLabels("method", "GET").
//	    Add(1)
//
//	// Assert in tests
//	value := adapter.GetCounter("test_requests", map[string]string{"method": "GET"})
//	require.Equal(t, 1.0, value)
package memory
