// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"github.com/altessa-s/go-atlas/observability/metrics"
)

// mongoMetrics holds all Prometheus metrics for the MongoDB client.
// When no [metrics.Collector] is provided, [metrics.Noop] is used and
// all methods become zero-cost no-ops.
type mongoMetrics struct {
	connectDuration     metrics.Timer
	pingRetries         metrics.Counter
	transactionDuration metrics.Timer
	transactionErrors   metrics.Counter
	operationsTotal     metrics.Counter
	operationDuration   metrics.Timer
}

func newMongoMetrics(c metrics.Collector) *mongoMetrics {
	if c == nil {
		c = metrics.Noop()
	}

	scoped := c.WithSubsystem("mongo")

	return &mongoMetrics{
		connectDuration: scoped.MustTimer(metrics.HistogramOpts{
			MetricOpts: metrics.MetricOpts{
				Name: "connect_duration_seconds",
				Help: "Duration of MongoDB connect operations in seconds.",
			},
		}),
		pingRetries: scoped.MustCounter(metrics.MetricOpts{
			Name: "ping_retries_total",
			Help: "Total number of MongoDB ping retry attempts.",
		}),
		transactionDuration: scoped.MustTimer(metrics.HistogramOpts{
			MetricOpts: metrics.MetricOpts{
				Name:       "transaction_duration_seconds",
				Help:       "Duration of MongoDB transactions in seconds.",
				LabelNames: []string{"collection"},
			},
		}),
		transactionErrors: scoped.MustCounter(metrics.MetricOpts{
			Name: "transaction_errors_total",
			Help: "Total number of failed MongoDB transactions.",
		}),
		operationsTotal: scoped.MustCounter(metrics.MetricOpts{
			Name:       "operations_total",
			Help:       "Total number of MongoDB CRUD operations.",
			LabelNames: []string{"op", "collection"},
		}),
		operationDuration: scoped.MustTimer(metrics.HistogramOpts{
			MetricOpts: metrics.MetricOpts{
				Name:       "operation_duration_seconds",
				Help:       "Duration of MongoDB operations in seconds.",
				LabelNames: []string{"op", "collection"},
			},
		}),
	}
}
