// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filter_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/filter"
)

func BenchmarkRegexCacheSaturation(b *testing.B) {
	e, err := filter.NewEvaluator()
	require.NoError(b, err)
	data := map[string]any{"name": "audit0"}
	makeNode := func(pattern string) filter.Node {
		return &filter.CallNode{Op: filter.OpMatches, Target: &filter.IdentNode{Name: "name"}, Args: []filter.Node{&filter.LiteralNode{Value: pattern}}}
	}
	for i := range 256 {
		_, err := e.Evaluate(makeNode(fmt.Sprintf("^audit%d$", i)), data)
		require.NoError(b, err)
	}
	for _, tc := range []struct{ name, pattern string }{{"admitted", "^audit0$"}, {"after_capacity", "^audit999$"}} {
		b.Run(tc.name, func(b *testing.B) {
			node := makeNode(tc.pattern)
			b.ReportAllocs()
			for b.Loop() {
				_, _ = e.Evaluate(node, data)
			}
		})
	}
}
