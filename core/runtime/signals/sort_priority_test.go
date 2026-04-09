// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package signals

import (
	"context"
	"math/rand/v2"
	"os"
	"slices"
	"strconv"
	"testing"
	"time"
)

// noopHandler is a trivial ContextHandler used purely to give handlerEntry a
// non-nil callback field. The sort routines under test do not invoke it.
func noopHandler(context.Context, os.Signal) error { return nil }

// makeHandlersWithPriorities builds handler entries with the given
// priorities in order. The timeout field is used as an identity tag so
// stability assertions can recover each entry's original position.
func makeHandlersWithPriorities(priorities []Priority) []handlerEntry {
	out := make([]handlerEntry, len(priorities))
	for i, p := range priorities {
		out[i] = handlerEntry{
			contextHandler: noopHandler,
			priority:       p,
			timeout:        time.Duration(i+1) * time.Microsecond, // unique per entry
		}
	}
	return out
}

// assertSortedDescending fails the test if handlers is not in descending
// priority order.
func assertSortedDescending(t *testing.T, handlers []handlerEntry) {
	t.Helper()
	for i := 1; i < len(handlers); i++ {
		if handlers[i-1].priority < handlers[i].priority {
			t.Fatalf("not descending at i=%d: %d < %d",
				i, handlers[i-1].priority, handlers[i].priority)
		}
	}
}

// assertStable fails the test if, within runs of equal priority, the
// original identity tags are not in ascending order (i.e. registration
// order is not preserved).
func assertStable(t *testing.T, handlers []handlerEntry) {
	t.Helper()
	for i := 1; i < len(handlers); i++ {
		if handlers[i-1].priority == handlers[i].priority &&
			handlers[i-1].timeout > handlers[i].timeout {
			t.Fatalf("stability broken at i=%d: priority=%d, timeouts %v > %v",
				i, handlers[i].priority, handlers[i-1].timeout, handlers[i].timeout)
		}
	}
}

// TestSortHandlers_DescendingAcrossAlgorithms exercises the dispatcher at a
// spread of sizes, covering both the insertion-sort path (n ≤
// insertionSortThreshold) and the counting-sort path above it. Priorities
// include values outside the documented [1..100] band so we verify the
// counting sort handles arbitrary int priorities.
func TestSortHandlers_DescendingAcrossAlgorithms(t *testing.T) {
	s := &Signal{}

	for _, n := range []int{0, 1, 2, 5, insertionSortThreshold, insertionSortThreshold + 1, 50, 100, 200} {
		t.Run("n="+strconv.Itoa(n), func(t *testing.T) {
			rng := rand.New(rand.NewPCG(uint64(n), 0xC0FFEE))
			priorities := make([]Priority, n)
			for i := range priorities {
				priorities[i] = Priority(rng.IntN(200) - 50) // includes <1 and >100
			}
			handlers := makeHandlersWithPriorities(priorities)

			s.sortHandlers(handlers)

			assertSortedDescending(t, handlers)
		})
	}
}

// TestSortHandlers_Stable asserts that both sort paths preserve registration
// order within a priority level.
func TestSortHandlers_Stable(t *testing.T) {
	s := &Signal{}

	for _, n := range []int{10, insertionSortThreshold, insertionSortThreshold + 1, 50, 100} {
		t.Run("n="+strconv.Itoa(n), func(t *testing.T) {
			// Use only three priority levels so stability is meaningful:
			// each level will contain many entries that must retain
			// registration order.
			priorities := make([]Priority, n)
			for i := range priorities {
				switch i % 3 {
				case 0:
					priorities[i] = PriorityHigh
				case 1:
					priorities[i] = PriorityNormal
				case 2:
					priorities[i] = PriorityLow
				}
			}
			handlers := makeHandlersWithPriorities(priorities)

			s.sortHandlers(handlers)

			assertSortedDescending(t, handlers)
			assertStable(t, handlers)
		})
	}
}

// TestSortHandlersByPriorityCounting_HandlesAllSamePriority verifies the
// degenerate case where every handler shares a priority: the counting sort
// should be a no-op that preserves order.
func TestSortHandlersByPriorityCounting_HandlesAllSamePriority(t *testing.T) {
	s := &Signal{}

	const n = 50
	priorities := make([]Priority, n)
	for i := range priorities {
		priorities[i] = PriorityNormal
	}
	handlers := makeHandlersWithPriorities(priorities)

	ok := s.sortHandlersByPriorityCounting(handlers)
	if !ok {
		t.Fatal("counting sort unexpectedly refused all-same-priority input")
	}

	// Original registration order must be preserved.
	for i, h := range handlers {
		want := time.Duration(i+1) * time.Microsecond
		if h.timeout != want {
			t.Errorf("entry %d: timeout=%v, want %v (registration order broken)", i, h.timeout, want)
		}
	}
}

// TestSortHandlersByPriorityCounting_HandlesNegativePriorities verifies the
// min/max pass correctly tracks negative priorities and the counting sort
// produces the right descending order.
func TestSortHandlersByPriorityCounting_HandlesNegativePriorities(t *testing.T) {
	s := &Signal{}

	handlers := makeHandlersWithPriorities([]Priority{
		-10, 0, 5, -5, 10, -10, 0, 5, // duplicates present for stability check
	})

	ok := s.sortHandlersByPriorityCounting(handlers)
	if !ok {
		t.Fatal("counting sort refused valid negative-priority input")
	}

	assertSortedDescending(t, handlers)
	assertStable(t, handlers)
}

// TestSortHandlersByPriorityCounting_RejectsSparseRange verifies the
// guardrail: when the priority range is wider than countingSortMaxRange,
// counting sort declines and the caller falls back.
func TestSortHandlersByPriorityCounting_RejectsSparseRange(t *testing.T) {
	s := &Signal{}

	// Two handlers with priorities 2000 apart — well past countingSortMaxRange.
	handlers := makeHandlersWithPriorities([]Priority{0, 2000})
	original := slices.Clone(handlers)

	if ok := s.sortHandlersByPriorityCounting(handlers); ok {
		t.Fatal("counting sort accepted pathologically sparse range; guardrail missing")
	}
	// The function must not mutate the slice when it refuses.
	if !slices.EqualFunc(handlers, original, func(a, b handlerEntry) bool {
		return a.priority == b.priority && a.timeout == b.timeout
	}) {
		t.Error("counting sort mutated handlers before rejecting")
	}

	// The dispatcher must fall back and still produce sorted output.
	s.sortHandlers(handlers)
	assertSortedDescending(t, handlers)
}

// TestSortHandlersByPriorityCounting_RejectsSparseRelativeToN verifies the
// density guardrail: when the range is more than 8× the handler count,
// counting sort declines even if the range is under the absolute ceiling.
func TestSortHandlersByPriorityCounting_RejectsSparseRelativeToN(t *testing.T) {
	s := &Signal{}

	// 3 handlers with range 100 → 100 > 8*3, so counting sort should
	// refuse and return false.
	handlers := makeHandlersWithPriorities([]Priority{0, 50, 100})
	if ok := s.sortHandlersByPriorityCounting(handlers); ok {
		t.Fatal("counting sort accepted sparse-relative-to-n input; density guard missing")
	}
}

// BenchmarkSortHandlers_Dispatch benchmarks the real user-facing entry
// point — the dispatcher picks insertion sort or counting sort based on n.
// Compare against BenchmarkSortHandlersByPriority_Insertion to see where
// counting sort starts to win.
func BenchmarkSortHandlers_Dispatch(b *testing.B) {
	s := &Signal{}

	for _, n := range []int{5, 10, 25, 50, 100, 200, 500} {
		b.Run("n="+strconv.Itoa(n), func(b *testing.B) {
			base := seededPriorities(n, 1)
			buf := make([]handlerEntry, n)

			b.ReportAllocs()
			for b.Loop() {
				fillHandlers(buf, base)
				s.sortHandlers(buf)
			}
		})
	}
}

// BenchmarkSortHandlersByPriority_Insertion isolates the insertion-sort
// implementation so we can measure head-to-head against counting sort.
func BenchmarkSortHandlersByPriority_Insertion(b *testing.B) {
	s := &Signal{}

	for _, n := range []int{5, 10, 25, 50, 100, 200, 500} {
		b.Run("n="+strconv.Itoa(n), func(b *testing.B) {
			base := seededPriorities(n, 1)
			buf := make([]handlerEntry, n)

			b.ReportAllocs()
			for b.Loop() {
				fillHandlers(buf, base)
				s.sortHandlersByPriority(buf)
			}
		})
	}
}

// BenchmarkSortHandlersByPriority_Counting isolates the counting-sort
// implementation so we can measure head-to-head against insertion sort at
// sizes where it is the dispatcher's choice. Sizes where the density guard
// refuses the input (range > 8×n, hit at very small n with [1..100]
// priorities) are skipped, since counting sort is not the intended
// algorithm for those cases.
func BenchmarkSortHandlersByPriority_Counting(b *testing.B) {
	s := &Signal{}

	for _, n := range []int{5, 10, 25, 50, 100, 200, 500} {
		b.Run("n="+strconv.Itoa(n), func(b *testing.B) {
			base := seededPriorities(n, 1)
			buf := make([]handlerEntry, n)

			// Probe once: if the guardrail refuses this size+distribution,
			// counting sort is by-design not applicable here.
			fillHandlers(buf, base)
			if !s.sortHandlersByPriorityCounting(buf) {
				b.Skipf("counting sort declines n=%d with [1..100] priorities (density guard)", n)
			}

			b.ReportAllocs()
			for b.Loop() {
				fillHandlers(buf, base)
				_ = s.sortHandlersByPriorityCounting(buf)
			}
		})
	}
}

// seededPriorities produces n priorities drawn from the documented [1..100]
// range using a fixed seed so runs are reproducible.
func seededPriorities(n int, seed uint64) []Priority {
	rng := rand.New(rand.NewPCG(seed, 0xABCD))
	out := make([]Priority, n)
	for i := range out {
		out[i] = Priority(rng.IntN(100) + 1)
	}
	return out
}

// fillHandlers refills buf with handlerEntry records from the priorities
// slice. Used inside benchmarks so every iteration sorts fresh data.
func fillHandlers(buf []handlerEntry, priorities []Priority) {
	for i, p := range priorities {
		buf[i] = handlerEntry{contextHandler: noopHandler, priority: p}
	}
}
