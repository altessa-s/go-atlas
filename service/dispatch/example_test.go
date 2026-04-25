// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package dispatch_test

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/altessa-s/go-atlas/service/dispatch"
)

// record is the domain type the engine batches and persists.
type record struct {
	ID    int
	Label string
}

// recordSink is a trivial [dispatch.Sink] that captures items in memory.
// Real implementations would write to a database, message broker, or
// another durable store.
type recordSink struct {
	mu    sync.Mutex
	items []record
}

func (s *recordSink) StoreBatch(_ context.Context, items []record) error {
	s.mu.Lock()
	s.items = append(s.items, items...)
	s.mu.Unlock()
	return nil
}

// Example shows the typical lifecycle of an in-memory [dispatch.Engine]:
// construct with a sink, Start, Submit on the hot path, then Shutdown for
// a graceful drain. For crash safety, pass [dispatch.WithWAL] — see the
// package tests for a WAL-enabled example that uses t.TempDir.
func Example() {
	sink := &recordSink{}

	eng, err := dispatch.NewEngine[record](sink,
		dispatch.WithBatchSize[record](10),
		dispatch.WithFlushInterval[record](10*time.Millisecond),
	)
	if err != nil {
		fmt.Println("new engine:", err)
		return
	}
	if err := eng.Start(); err != nil {
		fmt.Println("start:", err)
		return
	}

	// Hot path: Submit is non-blocking.
	for i := 1; i <= 5; i++ {
		eng.Submit(record{ID: i, Label: fmt.Sprintf("item-%d", i)})
	}

	// Graceful shutdown drains the queue.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := eng.Shutdown(ctx); err != nil {
		fmt.Println("shutdown:", err)
		return
	}

	// Results are observable on the sink. Sort for deterministic output.
	sink.mu.Lock()
	defer sink.mu.Unlock()
	sort.Slice(sink.items, func(i, j int) bool { return sink.items[i].ID < sink.items[j].ID })
	for _, it := range sink.items {
		fmt.Println(it.ID, it.Label)
	}
	// Output:
	// 1 item-1
	// 2 item-2
	// 3 item-3
	// 4 item-4
	// 5 item-5
}
