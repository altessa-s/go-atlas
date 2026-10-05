// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package facade_test

import (
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/data/probfilter/internal/facade"
)

func BenchmarkCoordinator_Add(b *testing.B) {
	var c facade.Coordinator
	write := func() error { return nil }
	for b.Loop() {
		_ = c.Add(b.Context(), "value", write)
	}
}

func BenchmarkBase_MightExist(b *testing.B) {
	base := facade.NewBase(newMemStore())
	for b.Loop() {
		_, _ = base.MightExist(b.Context(), "value")
	}
}

func BenchmarkObserverSlot_Lookup(b *testing.B) {
	lookup := func() (bool, error) { return true, nil }

	b.Run("no observer", func(b *testing.B) {
		var slot facade.ObserverSlot
		for b.Loop() {
			_, _ = slot.Lookup(lookup)
		}
	})

	b.Run("observer", func(b *testing.B) {
		var slot facade.ObserverSlot
		slot.Set(nopObserver{})
		for b.Loop() {
			_, _ = slot.Lookup(lookup)
		}
	})
}

type nopObserver struct{}

func (nopObserver) ObserveLookup(bool, error, time.Duration) {}
func (nopObserver) ObserveAdd(int)                           {}
func (nopObserver) ObserveRebuild(time.Duration, error)      {}
