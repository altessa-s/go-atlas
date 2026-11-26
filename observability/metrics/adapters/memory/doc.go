// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package memory provides an in-memory [adapters.Adapter] implementation
// for testing. Metric values are accumulated in goroutine-safe maps
// and exposed via lookup methods.
//
// # Usage
//
//	adapter := memory.New()
//	collector := metrics.New(metrics.WithAdapter(adapter), metrics.WithServiceName("test"))
//
//	// Record metrics
//	metrics.MustCounter(collector, metrics.MetricOpts{
//	    Name:       "requests_total",
//	    Help:       "Total requests",
//	    LabelNames: []string{"method"},
//	}).WithLabels(metrics.Labels{"method": "GET"}).Inc()
//
//	// Assert in tests
//	value := adapter.CounterValue("test_requests_total", map[string]string{"method": "GET"})
//	require.Equal(t, 1.0, value)
package memory
