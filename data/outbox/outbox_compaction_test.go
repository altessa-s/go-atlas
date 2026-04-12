// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outbox

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

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

	require.Equal(t, 1, len(toPublish))
	require.True(t, len(toPublish) > 0)
	require.Equal(t, "3", toPublish[0].Id)

	require.Equal(t, 2, len(toSkip))
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

	require.Equal(t, 3, len(toPublish))

	require.Equal(t, 3, len(toSkip))

	// Verify that the latest event for each key is published
	publishedIds := make(map[string]bool)
	for _, e := range toPublish {
		publishedIds[e.Id] = true
	}

	require.True(t, publishedIds["3"], "expected event 3 to be published")
	require.True(t, publishedIds["5"], "expected event 5 to be published")
	require.True(t, publishedIds["6"], "expected event 6 to be published")
}

func TestCompactEventsByKey_EmptySlice(t *testing.T) {
	o := &Outbox{}

	toPublish, toSkip := o.compactEventsByKey([]Event{})

	require.Nil(t, toPublish)
	require.Nil(t, toSkip)
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
	require.Equal(t, 3, len(toPublish))

	// Should skip: 2 older orders events
	require.Equal(t, 2, len(toSkip))

	// Verify that event 3 (latest orders) is published
	publishedIds := make(map[string]bool)
	for _, e := range toPublish {
		publishedIds[e.Id] = true
	}

	require.True(t, publishedIds["3"], "expected event 3 (latest orders) to be published")

	// Verify that both users events are published (not compacted)
	require.True(t, publishedIds["4"], "expected users event 4 to be published")
	require.True(t, publishedIds["5"], "expected users event 5 to be published")
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
	require.Equal(t, 4, len(toPublish))

	// Nothing should be skipped
	require.Equal(t, 0, len(toSkip))
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
	require.Equal(t, 4, len(toPublish))

	// Should skip: 1 older orders + 1 older inventory = 2
	require.Equal(t, 2, len(toSkip))

	// Verify published events
	publishedIds := make(map[string]bool)
	for _, e := range toPublish {
		publishedIds[e.Id] = true
	}

	// Latest for compacted topics: 2 (orders.123), 5 (inventory.xyz)
	// All for non-compacted topics: 3, 6 (users.abc)
	require.True(t, publishedIds["2"], "expected event 2 (latest orders.123) to be published")
	require.True(t, publishedIds["5"], "expected event 5 (latest inventory.xyz) to be published")
	require.True(t, publishedIds["3"], "expected users.abc event 3 to be published")
	require.True(t, publishedIds["6"], "expected users.abc event 6 to be published")
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
	require.Equal(t, 2, len(toPublish))

	// Should skip older events: 2 events
	require.Equal(t, 2, len(toSkip))

	// Verify latest events are published
	publishedIds := make(map[string]bool)
	for _, e := range toPublish {
		publishedIds[e.Id] = true
	}

	require.True(t, publishedIds["2"], "expected event 2 (latest orders.123) to be published")
	require.True(t, publishedIds["4"], "expected event 4 (latest users.abc) to be published")
}

func TestSetSkippedStatus_SetsPublishedAt(t *testing.T) {
	e := &Event{
		Id:     "test-event",
		Status: StatusPending,
	}

	beforeSet := time.Now().UTC()
	e.setSkippedStatus()
	afterSet := time.Now().UTC()

	require.Equal(t, StatusSkipped, e.Status)

	require.Nil(t, e.LastError)

	// Verify PublishedAt is set and within reasonable time range
	require.False(t, e.PublishedAt.IsZero(), "expected PublishedAt to be set, but it's zero")

	require.False(t, e.PublishedAt.Before(beforeSet), "expected PublishedAt >= %v, got %v", beforeSet, e.PublishedAt)
	require.False(t, e.PublishedAt.After(afterSet), "expected PublishedAt <= %v, got %v", afterSet, e.PublishedAt)
}
