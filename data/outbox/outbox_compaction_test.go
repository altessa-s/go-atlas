// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outbox

import (
	"strings"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/core/collections/slices"
)

func TestCompactEventsByKey_SingleKeyMultipleEvents(t *testing.T) {
	o := &Outbox{}

	now := time.Now()
	events := []Event{
		{Id: "1", Key: "orders.123", CreatedAt: now.Add(-2 * time.Second)},
		{Id: "2", Key: "orders.123", CreatedAt: now.Add(-1 * time.Second)},
		{Id: "3", Key: "orders.123", CreatedAt: now},
	}

	toPublish, toSkip := o.compactEventsByKey(events)

	if len(toPublish) != 1 {
		t.Errorf("expected 1 event to publish, got %d", len(toPublish))
	}
	if len(toPublish) > 0 && toPublish[0].Id != "3" {
		t.Errorf("expected event 3 to be published, got %s", toPublish[0].Id)
	}

	if len(toSkip) != 2 {
		t.Errorf("expected 2 events to skip, got %d", len(toSkip))
	}
}

func TestCompactEventsByKey_MultipleKeysMultipleEvents(t *testing.T) {
	o := &Outbox{}

	now := time.Now()
	events := []Event{
		{Id: "1", Key: "topic1", CreatedAt: now.Add(-5 * time.Second)},
		{Id: "2", Key: "topic1", CreatedAt: now.Add(-4 * time.Second)},
		{Id: "3", Key: "topic1", CreatedAt: now.Add(-3 * time.Second)},
		{Id: "4", Key: "topic2", CreatedAt: now.Add(-2 * time.Second)},
		{Id: "5", Key: "topic2", CreatedAt: now.Add(-1 * time.Second)},
		{Id: "6", Key: "topic3", CreatedAt: now},
	}

	toPublish, toSkip := o.compactEventsByKey(events)

	if len(toPublish) != 3 {
		t.Errorf("expected 3 events to publish, got %d", len(toPublish))
	}

	if len(toSkip) != 3 {
		t.Errorf("expected 3 events to skip, got %d", len(toSkip))
	}

	// Verify that the latest event for each key is published
	publishedIds := make(map[string]bool)
	for _, e := range toPublish {
		publishedIds[e.Id] = true
	}

	if !publishedIds["3"] || !publishedIds["5"] || !publishedIds["6"] {
		t.Errorf("expected events 3, 5, 6 to be published, got: %v", toPublish)
	}
}

func TestCompactEventsByKey_EmptySlice(t *testing.T) {
	o := &Outbox{}

	toPublish, toSkip := o.compactEventsByKey([]Event{})

	if toPublish != nil {
		t.Errorf("expected nil toPublish, got %v", toPublish)
	}
	if toSkip != nil {
		t.Errorf("expected nil toSkip, got %v", toSkip)
	}
}

func TestCompactEventsByKey_WithFilter_MatchingSome(t *testing.T) {
	o := &Outbox{
		compaction: true,
		compactionFilter: func(topic string) bool {
			return strings.HasPrefix(topic, "orders.")
		},
	}

	now := time.Now()
	events := []Event{
		{Id: "1", Key: "orders.123", CreatedAt: now.Add(-4 * time.Second)},
		{Id: "2", Key: "orders.123", CreatedAt: now.Add(-3 * time.Second)},
		{Id: "3", Key: "orders.123", CreatedAt: now.Add(-2 * time.Second)},
		{Id: "4", Key: "users.abc", CreatedAt: now.Add(-1 * time.Second)},
		{Id: "5", Key: "users.abc", CreatedAt: now},
	}

	toPublish, toSkip := o.compactEventsByKey(events)

	// Should publish: 1 orders event (latest) + 2 users events (not compacted) = 3
	if len(toPublish) != 3 {
		t.Errorf("expected 3 events to publish, got %d", len(toPublish))
	}

	// Should skip: 2 older orders events
	if len(toSkip) != 2 {
		t.Errorf("expected 2 events to skip, got %d", len(toSkip))
	}

	// Verify that event 3 (latest orders) is published
	publishedIds := make(map[string]bool)
	for _, e := range toPublish {
		publishedIds[e.Id] = true
	}

	if !publishedIds["3"] {
		t.Errorf("expected event 3 (latest orders) to be published")
	}

	// Verify that both users events are published (not compacted)
	if !publishedIds["4"] || !publishedIds["5"] {
		t.Errorf("expected both users events (4, 5) to be published")
	}
}

func TestCompactEventsByKey_WithFilter_NoMatches(t *testing.T) {
	o := &Outbox{
		compaction: true,
		compactionFilter: func(topic string) bool {
			return topic == "specific.topic"
		},
	}

	now := time.Now()
	events := []Event{
		{Id: "1", Key: "orders.123", CreatedAt: now.Add(-3 * time.Second)},
		{Id: "2", Key: "orders.123", CreatedAt: now.Add(-2 * time.Second)},
		{Id: "3", Key: "users.abc", CreatedAt: now.Add(-1 * time.Second)},
		{Id: "4", Key: "users.abc", CreatedAt: now},
	}

	toPublish, toSkip := o.compactEventsByKey(events)

	// All events should be published (no key matches filter)
	if len(toPublish) != 4 {
		t.Errorf("expected 4 events to publish, got %d", len(toPublish))
	}

	// Nothing should be skipped
	if len(toSkip) != 0 {
		t.Errorf("expected 0 events to skip, got %d", len(toSkip))
	}
}

func TestCompactEventsByKey_WithFilter_Whitelist(t *testing.T) {
	compactKeys := []string{"orders.123", "inventory.xyz"}

	o := &Outbox{
		compaction: true,
		compactionFilter: func(topic string) bool {
			return slices.Any(compactKeys, func(t string) bool { return t == topic })
		},
	}

	now := time.Now()
	events := []Event{
		{Id: "1", Key: "orders.123", CreatedAt: now.Add(-5 * time.Second)},
		{Id: "2", Key: "orders.123", CreatedAt: now.Add(-4 * time.Second)},
		{Id: "3", Key: "users.abc", CreatedAt: now.Add(-3 * time.Second)},
		{Id: "4", Key: "inventory.xyz", CreatedAt: now.Add(-2 * time.Second)},
		{Id: "5", Key: "inventory.xyz", CreatedAt: now.Add(-1 * time.Second)},
		{Id: "6", Key: "users.abc", CreatedAt: now},
	}

	toPublish, toSkip := o.compactEventsByKey(events)

	// Should publish: 1 orders (latest) + 2 users (not compacted) + 1 inventory (latest) = 4
	if len(toPublish) != 4 {
		t.Errorf("expected 4 events to publish, got %d", len(toPublish))
	}

	// Should skip: 1 older orders + 1 older inventory = 2
	if len(toSkip) != 2 {
		t.Errorf("expected 2 events to skip, got %d", len(toSkip))
	}

	// Verify published events
	publishedIds := make(map[string]bool)
	for _, e := range toPublish {
		publishedIds[e.Id] = true
	}

	// Latest for compacted topics: 2 (orders.123), 5 (inventory.xyz)
	// All for non-compacted topics: 3, 6 (users.abc)
	if !publishedIds["2"] {
		t.Errorf("expected event 2 (latest orders.123) to be published")
	}
	if !publishedIds["5"] {
		t.Errorf("expected event 5 (latest inventory.xyz) to be published")
	}
	if !publishedIds["3"] || !publishedIds["6"] {
		t.Errorf("expected both users.abc events (3, 6) to be published")
	}
}

func TestCompactEventsByKey_AllKeysCompacted(t *testing.T) {
	// When compactionFilter is nil, all keys should be compacted
	o := &Outbox{
		compaction:       true,
		compactionFilter: nil, // nil = all keys compacted
	}

	now := time.Now()
	events := []Event{
		{Id: "1", Key: "orders.123", CreatedAt: now.Add(-3 * time.Second)},
		{Id: "2", Key: "orders.123", CreatedAt: now.Add(-2 * time.Second)},
		{Id: "3", Key: "users.abc", CreatedAt: now.Add(-1 * time.Second)},
		{Id: "4", Key: "users.abc", CreatedAt: now},
	}

	toPublish, toSkip := o.compactEventsByKey(events)

	// Should publish latest for each key: 2 events
	if len(toPublish) != 2 {
		t.Errorf("expected 2 events to publish, got %d", len(toPublish))
	}

	// Should skip older events: 2 events
	if len(toSkip) != 2 {
		t.Errorf("expected 2 events to skip, got %d", len(toSkip))
	}

	// Verify latest events are published
	publishedIds := make(map[string]bool)
	for _, e := range toPublish {
		publishedIds[e.Id] = true
	}

	if !publishedIds["2"] {
		t.Errorf("expected event 2 (latest orders.123) to be published")
	}
	if !publishedIds["4"] {
		t.Errorf("expected event 4 (latest users.abc) to be published")
	}
}

func TestSetSkippedStatus_SetsPublishedAt(t *testing.T) {
	e := &Event{
		Id:     "test-event",
		Status: StatusPending,
	}

	beforeSet := time.Now().UTC()
	e.setSkippedStatus()
	afterSet := time.Now().UTC()

	if e.Status != StatusSkipped {
		t.Errorf("expected status to be StatusSkipped, got %s", e.Status)
	}

	if e.LastError != nil {
		t.Errorf("expected LastError to be nil, got %v", e.LastError)
	}

	// Verify PublishedAt is set and within reasonable time range
	if e.PublishedAt.IsZero() {
		t.Errorf("expected PublishedAt to be set, but it's zero")
	}

	if e.PublishedAt.Before(beforeSet) || e.PublishedAt.After(afterSet) {
		t.Errorf("expected PublishedAt to be between %v and %v, got %v",
			beforeSet, afterSet, e.PublishedAt)
	}
}
