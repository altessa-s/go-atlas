// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/data/outbox"
	"github.com/altessa-s/go-atlas/data/outbox/store/sqldb"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

func benchStore(b *testing.B) *sqldb.Store {
	b.Helper()
	db, _ := testhelpers.NewFakeSQL(b, nil)
	store, err := sqldb.New(db, sqldb.DialectMySQL)
	if err != nil {
		b.Fatal(err)
	}
	return store
}

// BenchmarkSaveEvents measures building the multi-row INSERT for a batch; the
// fake driver makes the round trip itself negligible.
func BenchmarkSaveEvents(b *testing.B) {
	store := benchStore(b)
	events := make([]outbox.Event, 100)
	for i := range events {
		events[i] = outbox.Event{Id: strconv.Itoa(i), Key: "orders.created", Payload: []byte(`{"n":1}`),
			Status: outbox.StatusPending, CreatedAt: time.Now()}
	}

	b.ReportAllocs()
	for b.Loop() {
		if err := store.SaveEvents(b.Context(), events...); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkUpdateEvents measures the per-event fenced writes of one dispatch
// cycle's results.
func BenchmarkUpdateEvents(b *testing.B) {
	store := benchStore(b)
	events := make([]outbox.Event, 100)
	for i := range events {
		events[i] = outbox.Event{Id: strconv.Itoa(i), Status: outbox.StatusFailed, Attempts: 1, LockToken: "t", RetryAfter: time.Second}
	}

	b.ReportAllocs()
	for b.Loop() {
		if err := store.UpdateEvents(b.Context(), events...); err != nil {
			b.Fatal(err)
		}
	}
}
