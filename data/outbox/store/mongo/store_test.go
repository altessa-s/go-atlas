// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outboxstore

import (
	"testing"
	"time"
)

func TestDefaultOptions(t *testing.T) {
	opts := defaultOptions()
	if opts.collectionName != DefaultCollectionName {
		t.Fatalf("collectionName = %q, want %q", opts.collectionName, DefaultCollectionName)
	}
	if opts.indexTimeout != DefaultIndexCreateTimeout {
		t.Fatalf("indexTimeout = %v, want %v", opts.indexTimeout, DefaultIndexCreateTimeout)
	}
	if opts.ctx != nil {
		t.Fatal("ctx should be nil by default")
	}
}

func TestWithCollectionName_String(t *testing.T) {
	opts := newOptions(WithCollectionName("my_events"))
	if opts.collectionName != "my_events" {
		t.Fatalf("collectionName = %q", opts.collectionName)
	}
}

func TestWithCollectionName_StringPtr(t *testing.T) {
	name := "ptr_events"
	opts := newOptions(WithCollectionName(&name))
	if opts.collectionName != "ptr_events" {
		t.Fatalf("collectionName = %q", opts.collectionName)
	}
}

func TestWithCollectionName_NilPtr(t *testing.T) {
	opts := newOptions(WithCollectionName[*string](nil))
	if opts.collectionName != DefaultCollectionName {
		t.Fatalf("nil ptr should keep default, got %q", opts.collectionName)
	}
}

func TestWithCollectionName_Trimmed(t *testing.T) {
	opts := newOptions(WithCollectionName("  spaced  "))
	if opts.collectionName != "spaced" {
		t.Fatalf("collectionName = %q", opts.collectionName)
	}
}

func TestWithContext(t *testing.T) {
	ctx := t.Context()
	opts := newOptions(WithContext(ctx))
	if opts.ctx != ctx {
		t.Fatal("ctx not set")
	}
}

func TestWithIndexCreateTimeout(t *testing.T) {
	opts := newOptions(WithIndexCreateTimeout(30 * time.Second))
	if opts.indexTimeout != 30*time.Second {
		t.Fatalf("indexTimeout = %v", opts.indexTimeout)
	}
}

func TestWithIndexCreateTimeout_Negative(t *testing.T) {
	opts := newOptions(WithIndexCreateTimeout(-1))
	if opts.indexTimeout != DefaultIndexCreateTimeout {
		t.Fatalf("negative should keep default, got %v", opts.indexTimeout)
	}
}

func TestWithIndexCreateTimeout_Zero(t *testing.T) {
	opts := newOptions(WithIndexCreateTimeout(0))
	if opts.indexTimeout != DefaultIndexCreateTimeout {
		t.Fatalf("zero should keep default, got %v", opts.indexTimeout)
	}
}

func TestConstants(t *testing.T) {
	if DefaultCollectionName == "" {
		t.Fatal("DefaultCollectionName is empty")
	}
	if DefaultIndexCreateTimeout <= 0 {
		t.Fatalf("DefaultIndexCreateTimeout = %v", DefaultIndexCreateTimeout)
	}
}

func TestEventsIndexes(t *testing.T) {
	if len(eventsIndexes) == 0 {
		t.Fatal("eventsIndexes is empty")
	}
	for i, idx := range eventsIndexes {
		if idx.Keys == nil {
			t.Fatalf("index %d has nil Keys", i)
		}
	}
}

func TestCollectionFieldConstants(t *testing.T) {
	fields := []string{
		collectionFieldId,
		collectionFieldPublishedAt,
		collectionFieldStatus,
		collectionFieldLockedOn,
		collectionFieldCreatedAt,
		collectionFieldLastAttemptOn,
		collectionFieldLastAttempts,
		collectionFieldExpiresAt,
	}
	seen := make(map[string]bool)
	for _, f := range fields {
		if f == "" {
			t.Fatal("empty field constant")
		}
		if seen[f] {
			t.Fatalf("duplicate field constant: %q", f)
		}
		seen[f] = true
	}
}
