// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package brokerit

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/altessa-s/go-atlas/transport/broker"
	"github.com/altessa-s/go-atlas/transport/broker/msg"
)

// Reaction is what a Recorder does with a delivery once it has recorded it.
// attempt counts the deliveries of the same payload, starting at 1.
type Reaction func(ctx context.Context, m *msg.Message, attempt int)

// Ack is the Reaction of a handler that always succeeds.
func Ack(_ context.Context, m *msg.Message, _ int) { _ = m.Ack() }

// Delivery is one handler invocation.
type Delivery struct {
	Payload string
	Meta    msg.Meta
	At      time.Time
}

// Recorder is a [broker.SubscriberHandler] that records every delivery and
// then hands it to its Reaction. Safe for concurrent use.
type Recorder struct {
	topic string
	react Reaction

	mu         sync.Mutex
	deliveries []Delivery
	attempts   map[string]int
}

var _ broker.SubscriberHandler = (*Recorder)(nil)

// NewRecorder returns a Recorder subscribed to topic.
func NewRecorder(topic string, react Reaction) *Recorder {
	return &Recorder{topic: topic, react: react, attempts: make(map[string]int)}
}

// Topic reports the subject the Recorder subscribes to.
func (r *Recorder) Topic() string { return r.topic }

// Handle records the delivery, then runs the Reaction.
func (r *Recorder) Handle(ctx context.Context, m *msg.Message) {
	payload := string(m.Data)

	r.mu.Lock()
	r.attempts[payload]++
	attempt := r.attempts[payload]
	r.deliveries = append(r.deliveries, Delivery{Payload: payload, Meta: slices.Clone(m.Metadata), At: time.Now()})
	r.mu.Unlock()

	r.react(ctx, m, attempt)
}

// Count returns the number of deliveries recorded so far.
func (r *Recorder) Count() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return len(r.deliveries)
}

// Attempts returns how many times payload has been delivered.
func (r *Recorder) Attempts(payload string) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.attempts[payload]
}

// Distinct returns the number of different payloads delivered.
func (r *Recorder) Distinct() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return len(r.attempts)
}

// Deliveries returns a copy of the recording, in arrival order.
func (r *Recorder) Deliveries() []Delivery {
	r.mu.Lock()
	defer r.mu.Unlock()

	return slices.Clone(r.deliveries)
}

// DeliveriesOf returns the recorded deliveries of payload, in arrival order.
func (r *Recorder) DeliveriesOf(payload string) []Delivery {
	r.mu.Lock()
	defer r.mu.Unlock()

	var out []Delivery
	for _, d := range r.deliveries {
		if d.Payload == payload {
			out = append(out, d)
		}
	}

	return out
}
