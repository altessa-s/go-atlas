// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

import (
	"github.com/altessa-s/go-atlas/observability/metrics"
)

type outcome int

const (
	outcomeProcessed outcome = iota
	outcomeDuplicate
	outcomeBusy
	outcomeNoKey
	outcomeKeyError
	outcomeStorageError
	outcomeLockStolen
	numOutcomes
)

var outcomeNames = [...]string{
	outcomeProcessed:    "processed",
	outcomeDuplicate:    "duplicate",
	outcomeBusy:         "busy",
	outcomeNoKey:        "no_key",
	outcomeKeyError:     "key_error",
	outcomeStorageError: "storage_error",
	outcomeLockStolen:   "lock_stolen",
}

type brokerMetrics struct {
	counters [numOutcomes]metrics.Counter
}

func newBrokerMetrics(c metrics.Collector, consumer string) *brokerMetrics {
	if c == nil {
		c = metrics.Noop()
	}

	scoped := c.WithSubsystem("broker_idempotency")
	outcomes := scoped.MustCounter(metrics.MetricOpts{
		Name:       "outcomes_total",
		Help:       "Total number of broker idempotency outcomes by consumer and outcome.",
		LabelNames: []string{"consumer", "outcome"},
	})

	m := &brokerMetrics{}
	for i := range int(numOutcomes) {
		m.counters[i] = outcomes.WithLabels(metrics.Labels{
			"consumer": consumer,
			"outcome":  outcomeNames[i],
		})
	}
	return m
}

func (m *brokerMetrics) inc(o outcome) {
	if m != nil && int(o) < len(m.counters) && m.counters[o] != nil {
		m.counters[o].Inc()
	}
}
