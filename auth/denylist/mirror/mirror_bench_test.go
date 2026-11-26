// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mirror_test

import (
	"context"
	"iter"
	"strconv"
	"testing"

	"github.com/altessa-s/go-atlas/auth/denylist/mirror"
)

type sliceSource []string

func (s sliceSource) StreamValues(_ context.Context) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		for _, k := range s {
			if !yield(k, nil) {
				return
			}
		}
	}
}

func BenchmarkIsRevoked(b *testing.B) {
	keys := make(sliceSource, 10_000)
	for i := range keys {
		keys[i] = strconv.Itoa(i)
	}
	m := mirror.New(keys)
	if err := m.Refresh(b.Context()); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for b.Loop() {
		_, _ = m.IsRevoked("9999") // present
		_, _ = m.IsRevoked("absent")
	}
}

func BenchmarkRefresh(b *testing.B) {
	keys := make(sliceSource, 10_000)
	for i := range keys {
		keys[i] = strconv.Itoa(i)
	}
	m := mirror.New(keys)
	ctx := b.Context()

	b.ReportAllocs()
	for b.Loop() {
		if err := m.Refresh(ctx); err != nil {
			b.Fatal(err)
		}
	}
}
