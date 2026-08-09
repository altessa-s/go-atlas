// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outbox

import (
	"strconv"
	"testing"
	"time"
)

// compactionBatch builds a batch shaped like a real fetch: keys repeat, and the
// events arrive ordered by creation time.
func compactionBatch(size, keys int) []Event {
	now := time.Now().UTC()
	events := make([]Event, size)
	for i := range events {
		events[i] = Event{
			Id:        strconv.Itoa(i),
			Key:       "orders." + strconv.Itoa(i%keys),
			CreatedAt: now.Add(time.Duration(i) * time.Millisecond),
		}
	}
	return events
}

// Compaction runs over every fetched batch when enabled, and now also sorts it,
// so it sits on the dispatch hot path.
func BenchmarkCompactEventsByKey(b *testing.B) {
	o := &Outbox{compaction: true}
	events := compactionBatch(DefaultEventsBatchSize, DefaultEventsBatchSize/4)
	scratch := make([]Event, len(events))

	b.ReportAllocs()
	for b.Loop() {
		copy(scratch, events)
		o.compactEventsByKey(scratch)
	}
}

func BenchmarkCompactEventsByKey_WithFilter(b *testing.B) {
	o := &Outbox{
		compaction:       true,
		compactionFilter: func(k string) bool { return len(k) > 8 },
	}
	events := compactionBatch(DefaultEventsBatchSize, DefaultEventsBatchSize/4)
	scratch := make([]Event, len(events))

	b.ReportAllocs()
	for b.Loop() {
		copy(scratch, events)
		o.compactEventsByKey(scratch)
	}
}
