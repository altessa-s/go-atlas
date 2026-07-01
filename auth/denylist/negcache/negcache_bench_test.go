// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package negcache_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/auth/denylist/negcache"
)

// BenchmarkIsRevoked_Miss measures the hot path: a definite filter miss that is
// answered locally without touching the authoritative store.
func BenchmarkIsRevoked_Miss(b *testing.B) {
	c := negcache.New(newFakeFilter(), newFakeAuth())
	ctx := b.Context()

	b.ReportAllocs()
	for b.Loop() {
		_, _ = c.IsRevoked(ctx, "fresh")
	}
}

// BenchmarkIsRevoked_Hit measures the fall-through path: the filter reports the
// key present and the authoritative store is consulted for the exact answer.
func BenchmarkIsRevoked_Hit(b *testing.B) {
	filter := newFakeFilter()
	_ = filter.Add(b.Context(), "jti")
	c := negcache.New(filter, newFakeAuth("jti"))
	ctx := b.Context()

	b.ReportAllocs()
	for b.Loop() {
		_, _ = c.IsRevoked(ctx, "jti")
	}
}
