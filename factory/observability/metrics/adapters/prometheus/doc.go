// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory maps the Prometheus block of the metrics configuration to
// a Prometheus metrics adapter for factory/observability/metrics.
//
// The metrics factory knows no concrete adapter; register this one for the
// prometheus type:
//
//	collector, err := metricsfactory.New(&cfg.Metrics).
//		UseAdapter(observabilityconfig.MetricsTypePrometheus, promfactory.Builder()).
//		Build()
package factory
