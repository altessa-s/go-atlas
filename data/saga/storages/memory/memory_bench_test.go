// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/data/saga"
	"github.com/altessa-s/go-atlas/data/saga/storages/memory"
)

func BenchmarkUpdate(b *testing.B) {
	s := memory.New()
	ctx := b.Context()
	inst := &saga.Instance{ID: "a", Status: saga.StatusRunning, Data: []byte("payload")}
	if err := s.Create(ctx, inst); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for b.Loop() {
		// Re-read the current version each iteration so the CAS succeeds.
		cur, err := s.Get(ctx, "a")
		if err != nil {
			b.Fatal(err)
		}
		if err := s.Update(ctx, cur); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGet(b *testing.B) {
	s := memory.New()
	ctx := b.Context()
	if err := s.Create(ctx, &saga.Instance{ID: "a", Data: []byte("payload")}); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for b.Loop() {
		if _, err := s.Get(ctx, "a"); err != nil {
			b.Fatal(err)
		}
	}
}
