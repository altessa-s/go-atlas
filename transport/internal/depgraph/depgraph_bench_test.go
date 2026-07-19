// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package depgraph

import (
	"fmt"
	"log/slog"
	"testing"
)

func BenchmarkTopologicalSort(b *testing.B) {
	logger := slog.New(slog.DiscardHandler)

	for _, n := range []int{10, 100} {
		b.Run(fmt.Sprintf("nodes_%d", n), func(b *testing.B) {
			for b.Loop() {
				g := New[*benchItem](logger)
				for i := range n {
					name := fmt.Sprintf("item_%d", i)
					g.AddNode(name, &benchItem{name: name})
					if i > 0 {
						g.AddEdge(fmt.Sprintf("item_%d", i-1), name)
					}
				}
				g.TopologicalSort()
			}
		})
	}
}

func BenchmarkBuild(b *testing.B) {
	logger := slog.New(slog.DiscardHandler)
	items := make([]*benchDepItem, 20)
	for i := range items {
		var deps []string
		if i > 0 {
			deps = []string{fmt.Sprintf("item_%d", i-1)}
		}
		items[i] = &benchDepItem{name: fmt.Sprintf("item_%d", i), deps: deps}
	}

	for b.Loop() {
		Build(items, logger)
	}
}

type benchItem struct{ name string }

func (b *benchItem) Name() string { return b.name }

type benchDepItem struct {
	name string
	deps []string
}

func (b *benchDepItem) Name() string           { return b.name }
func (b *benchDepItem) Dependencies() []string { return b.deps }
