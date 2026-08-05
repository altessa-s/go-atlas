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
	processHooks.mu.Lock()
	processHooks.hooks = nil
	processHooks.once = sync.Once{}
	processHooks.mu.Unlock()

	noop := func(_ context.Context) error { return nil }
	for b.Loop() {
		OnShutdown(noop)
	}
}

func BenchmarkHookGroupOnShutdown(b *testing.B) {
	var group HookGroup

	noop := func(_ context.Context) error { return nil }
	for b.Loop() {
		group.OnShutdown(noop)
	}
}

// A group's Shutdown runs at most once, so the benchmark measures a full
// register-and-tear-down cycle rather than repeated Shutdown calls, which
// would be no-ops after the first.
func BenchmarkHookGroupLifecycle(b *testing.B) {
	noop := func(_ context.Context) error { return nil }
	ctx := b.Context()

	for b.Loop() {
		var group HookGroup
		for range 16 {
			group.OnShutdown(noop)
		}

		_ = group.Shutdown(ctx)
	}
}

func BenchmarkAddCleanup(b *testing.B) {
	for b.Loop() {
		obj := new(int)
		c := AddCleanup(obj, func(_ int) {}, 0)
		c.Stop()
	}
}
