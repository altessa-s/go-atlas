// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cache

import (
	"context"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/data/cache/providers"
)

type benchProvider struct {
	store map[string][]byte
}

func (b *benchProvider) Save(_ context.Context, key string, value []byte, _ time.Duration) error {
	b.store[key] = value
	return nil
}
func (b *benchProvider) Get(_ context.Context, key string) ([]byte, error) {
	v, ok := b.store[key]
	if !ok {
		return nil, providers.ErrMissing
	}
	return v, nil
}
func (b *benchProvider) Delete(_ context.Context, _ string) error         { return nil }
func (b *benchProvider) DeleteMany(_ context.Context, _ ...string) error  { return nil }
func (b *benchProvider) Exists(_ context.Context, _ string) (bool, error) { return true, nil }

func BenchmarkSave(b *testing.B) {
	p := &benchProvider{store: make(map[string][]byte)}
	c := New(p)
	ctx := b.Context()

	b.ResetTimer()
	for b.Loop() {
		_ = c.Save(ctx, "bench-key", "bench-value")
	}
}

func BenchmarkGetWithFallback(b *testing.B) {
	p := &benchProvider{store: make(map[string][]byte)}
	c := New(p)
	ctx := b.Context()
	// Pre-populate
	_ = c.Save(ctx, "bench-key", "value")

	b.ResetTimer()
	for b.Loop() {
		var result string
		_ = c.GetWithFallback(ctx, "bench-key", &result, func() (any, time.Duration, error) {
			return "fallback", TTLUseDefault, nil
		})
	}
}
