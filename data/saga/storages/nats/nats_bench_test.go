// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats_test

import (
	"strconv"
	"testing"

	"github.com/altessa-s/go-atlas/data/saga"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	natsstore "github.com/altessa-s/go-atlas/data/saga/storages/nats"
)

func BenchmarkCreate(b *testing.B) {
	ns := testhelpers.StartNATSServer(b)
	_, js := testhelpers.ConnectJetStream(b, ns)
	s, err := natsstore.New(js, natsstore.WithBucket("saga_bench_create"))
	if err != nil {
		b.Fatal(err)
	}
	ctx := b.Context()

	b.ReportAllocs()
	i := 0
	for b.Loop() {
		if err := s.Create(ctx, &saga.Instance{ID: strconv.Itoa(i), Status: saga.StatusRunning, Data: []byte(`{"n":1}`)}); err != nil {
			b.Fatal(err)
		}
		i++
	}
}

func BenchmarkGet(b *testing.B) {
	ns := testhelpers.StartNATSServer(b)
	_, js := testhelpers.ConnectJetStream(b, ns)
	s, err := natsstore.New(js, natsstore.WithBucket("saga_bench_get"))
	if err != nil {
		b.Fatal(err)
	}
	ctx := b.Context()
	if err := s.Create(ctx, &saga.Instance{ID: "a", Status: saga.StatusRunning, Data: []byte(`{"n":1}`)}); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for b.Loop() {
		if _, err := s.Get(ctx, "a"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUpdate(b *testing.B) {
	ns := testhelpers.StartNATSServer(b)
	_, js := testhelpers.ConnectJetStream(b, ns)
	s, err := natsstore.New(js, natsstore.WithBucket("saga_bench"))
	if err != nil {
		b.Fatal(err)
	}
	ctx := b.Context()
	if err := s.Create(ctx, &saga.Instance{ID: "a", Status: saga.StatusRunning, Data: []byte(`{"n":1}`)}); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for b.Loop() {
		// Re-read the current revision each iteration so the CAS succeeds.
		cur, err := s.Get(ctx, "a")
		if err != nil {
			b.Fatal(err)
		}
		if err := s.Update(ctx, cur); err != nil {
			b.Fatal(err)
		}
	}
}
