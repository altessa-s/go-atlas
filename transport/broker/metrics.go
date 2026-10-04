// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package broker

import (
	"sync"
	"sync/atomic"

	"github.com/altessa-s/go-atlas/observability/metrics"
)

// brokerMetrics holds all Prometheus metrics for the Broker.
// When no [metrics.Collector] is provided, [metrics.Noop] is used and
// all methods become zero-cost no-ops.
type brokerMetrics struct {
	messagesPublished metrics.Counter
	publishErrors     metrics.Counter
	publishDuration   metrics.Timer

	// published and errors cache label-bound counters per subject so the
	// per-publish path does not rebuild the label map on every message.
	// At most labelLimit distinct subjects get their own series; the rest
	// share OtherSubjectLabel.
	published  sync.Map // string → metrics.Counter
	errors     sync.Map // string → metrics.Counter
	labels     sync.Map // subject → struct{}: subjects admitted as label values
	labelCount atomic.Int64
	labelLimit int64
}

// OtherSubjectLabel is the subject label value shared by publish metrics once
// the subject label limit is reached.
const OtherSubjectLabel = "_other"

// subjectLabel returns subject while the label budget allows, else
// OtherSubjectLabel. Admission is first come, first served and permanent.
func (m *brokerMetrics) subjectLabel(subject string) string {
	if _, ok := m.labels.Load(subject); ok {
		return subject
	}
	if m.labelCount.Add(1) > m.labelLimit {
		m.labelCount.Add(-1)
		return OtherSubjectLabel
	}
	if _, loaded := m.labels.LoadOrStore(subject, struct{}{}); loaded {
		m.labelCount.Add(-1)
	}
	return subject
}

// publishedFor returns the messagesPublished counter bound to the subject,
// binding the label set on first use.
func (m *brokerMetrics) publishedFor(subject string) metrics.Counter {
	subject = m.subjectLabel(subject)
	if v, ok := m.published.Load(subject); ok {
		return v.(metrics.Counter) //nolint:errcheck
	}
	v, _ := m.published.LoadOrStore(subject, m.messagesPublished.WithLabels(metrics.Labels{"subject": subject}))
	return v.(metrics.Counter) //nolint:errcheck
}

// errorsFor returns the publishErrors counter bound to the subject,
// binding the label set on first use.
func (m *brokerMetrics) errorsFor(subject string) metrics.Counter {
	subject = m.subjectLabel(subject)
	if v, ok := m.errors.Load(subject); ok {
		return v.(metrics.Counter) //nolint:errcheck
	}
	v, _ := m.errors.LoadOrStore(subject, m.publishErrors.WithLabels(metrics.Labels{"subject": subject}))
	return v.(metrics.Counter) //nolint:errcheck
}

func newBrokerMetrics(c metrics.Collector, labelLimit int) *brokerMetrics {
	if c == nil {
		c = metrics.Noop()
	}

	scoped := c.WithSubsystem("broker")

	return &brokerMetrics{
		labelLimit: int64(labelLimit),
		messagesPublished: scoped.MustCounter(metrics.MetricOpts{
			Name:       "messages_published_total",
			Help:       "Total number of messages successfully published.",
			LabelNames: []string{"subject"},
		}),
		publishErrors: scoped.MustCounter(metrics.MetricOpts{
			Name:       "publish_errors_total",
			Help:       "Total number of message publish failures.",
			LabelNames: []string{"subject"},
		}),
		publishDuration: scoped.MustTimer(metrics.HistogramOpts{
			MetricOpts: metrics.MetricOpts{
				Name: "publish_duration_seconds",
				Help: "Duration of publish operations in seconds.",
			},
		}),
	}
}
