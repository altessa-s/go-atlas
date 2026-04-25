// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package runtime

import (
	"context"
	"sync"
	"testing"
)

func BenchmarkOnShutdown(b *testing.B) {
	shutdownMu.Lock()
	shutdownHooks = nil
	shutdownOnce = sync.Once{}
	shutdownMu.Unlock()

	noop := func(_ context.Context) error { return nil }
	for b.Loop() {
		OnShutdown(noop)
	}
}

func BenchmarkAddCleanup(b *testing.B) {
	for b.Loop() {
		obj := new(int)
		c := AddCleanup(obj, func(_ int) {}, 0)
		c.Stop()
	}
}
