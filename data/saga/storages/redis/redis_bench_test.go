// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"strconv"
	"testing"

	"github.com/altessa-s/go-atlas/data/saga"
)

func BenchmarkCreate(b *testing.B) {
	s := newStore(b)
	ctx := b.Context()

	b.ReportAllocs()
	i := 0
	for b.Loop() {
		if err := s.Create(ctx, instance(strconv.Itoa(i), saga.StatusRunning, baseTime)); err != nil {
			b.Fatal(err)
		}
		i++
	}
}

func BenchmarkGet(b *testing.B) {
	s := newStore(b)
	ctx := b.Context()
	if err := s.Create(ctx, instance("a", saga.StatusRunning, baseTime)); err != nil {
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
	s := newStore(b)
	ctx := b.Context()
	inst := instance("a", saga.StatusRunning, baseTime)
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

func BenchmarkFetchRecoverable(b *testing.B) {
	s := newStore(b)
	ctx := b.Context()
	if err := s.Create(ctx, instance("c", saga.StatusCompensating, baseTime)); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for b.Loop() {
		if _, err := s.FetchRecoverable(ctx, "", baseTime, 0); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkUpdate_LargePayload measures Update alone on an instance with a
// 64 KiB payload and some history, reusing the version Update writes back.
func BenchmarkUpdate_LargePayload(b *testing.B) {
	s := newStore(b)
	ctx := b.Context()
	inst := instance("a", saga.StatusRunning, baseTime)
	inst.Data = make([]byte, 64<<10)
	inst.Steps = make([]saga.StepRecord, 32)
	if err := s.Create(ctx, inst); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for b.Loop() {
		if err := s.Update(ctx, inst); err != nil {
			b.Fatal(err)
		}
	}
}
