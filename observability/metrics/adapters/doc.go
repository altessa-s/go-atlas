// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package adapters defines the [Adapter] interface for metrics backends.
// Adapters translate abstract metric operations from the parent metrics
// package to specific backend formats such as Prometheus, StatsD, or
// OpenTelemetry.
//
// Use [MultiAdapter] to broadcast to multiple backends simultaneously.
// For Prometheus specifically, see the prometheus sub-package.
//
// Implementations must be safe for concurrent use.
//
// # Example
//
//	type MyAdapter struct{}
//
//	func (a *MyAdapter) Name() string { return "myadapter" }
//	func (a *MyAdapter) Register(desc *Desc) error { return nil }
//	func (a *MyAdapter) RecordCounter(name string, labels map[string]string, delta float64) {}
//	func (a *MyAdapter) RecordGauge(name string, labels map[string]string, value float64) {}
//	func (a *MyAdapter) RecordHistogram(name string, labels map[string]string, value float64) {}
//	func (a *MyAdapter) Flush() error { return nil }
//	func (a *MyAdapter) Close() error { return nil }
package adapters
