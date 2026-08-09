// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outboxit

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/altessa-s/go-atlas/data/outbox"
)

// Delivery is one invocation of the outbox Handler — one attempt to hand an
// event to its destination.
type Delivery struct {
	EventID string
	Key     string
	Payload string
	Attempt uint32
	At      time.Time
}

// Recorder is a programmable outbox Handler that records every delivery it is
// asked to make.
//
// It records rather than counts: a duplicate is not merely a count that came
// out too high, it is two entries naming the same event, which is what the
// exclusivity scenarios need in order to say which event was delivered twice
// and how far apart. The verdict each delivery returns is programmable, so a
// test can model a broker that is down, one that rejects a specific message
// permanently, or one that is simply slow.
type Recorder struct {
	mu         sync.Mutex
	deliveries []Delivery
	respond    func(Delivery) error
}

// NewRecorder returns a Recorder whose deliveries all succeed.
func NewRecorder() *Recorder { return &Recorder{} }

// Respond installs the verdict function consulted for every subsequent
// delivery. A nil function means every delivery succeeds. The function runs
// while the outbox holds the event's store lock, so blocking inside it models a
// slow destination.
func (r *Recorder) Respond(fn func(Delivery) error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.respond = fn
}

// Handler returns the outbox.Handler backed by this recorder.
func (r *Recorder) Handler() outbox.Handler {
	return func(_ context.Context, event outbox.Event) error {
		d := Delivery{
			EventID: event.Id,
			Key:     event.Key,
			Payload: string(event.Payload),
			Attempt: event.Attempts,
			At:      time.Now(),
		}

		r.mu.Lock()
		r.deliveries = append(r.deliveries, d)
		respond := r.respond
		r.mu.Unlock()

		// Called outside the lock so a blocking verdict cannot stall the
		// recording of a concurrent delivery — which is exactly the case the
		// contention scenarios are trying to observe.
		if respond == nil {
			return nil
		}
		return respond(d)
	}
}

// Deliveries returns the recording, oldest first.
func (r *Recorder) Deliveries() []Delivery {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Delivery(nil), r.deliveries...)
}

// Count returns the total number of delivery attempts.
func (r *Recorder) Count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.deliveries)
}

// CountOf returns how many times one event was handed over. Anything above one
// is a duplicate delivery.
func (r *Recorder) CountOf(eventID string) int {
	n := 0
	for _, d := range r.Deliveries() {
		if d.EventID == eventID {
			n++
		}
	}
	return n
}

// Payloads returns the payload of every delivery, in order.
func (r *Recorder) Payloads() []string {
	deliveries := r.Deliveries()
	out := make([]string, 0, len(deliveries))
	for _, d := range deliveries {
		out = append(out, d.Payload)
	}
	return out
}

// Duplicates returns the IDs of events delivered more than once, with their
// delivery counts.
func (r *Recorder) Duplicates() map[string]int {
	counts := make(map[string]int)
	for _, d := range r.Deliveries() {
		counts[d.EventID]++
	}
	for id, n := range counts {
		if n == 1 {
			delete(counts, id)
		}
	}
	return counts
}

// Timeline renders the recording for a failure message.
func (r *Recorder) Timeline() string {
	var b strings.Builder

	deliveries := r.Deliveries()
	fmt.Fprintf(&b, "deliveries: %d\n", len(deliveries))
	for _, d := range deliveries {
		fmt.Fprintf(&b, "  %s key=%s attempt=%d at=%s payload=%q\n",
			d.EventID, d.Key, d.Attempt, d.At.Format("15:04:05.000"), d.Payload)
	}
	for id, n := range r.Duplicates() {
		fmt.Fprintf(&b, "  DUPLICATE: %s delivered %d times\n", id, n)
	}

	return b.String()
}
