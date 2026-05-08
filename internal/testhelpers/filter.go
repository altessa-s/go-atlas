// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package testhelpers

import (
	"testing"

	"github.com/altessa-s/go-atlas/data/filter"
)

// MustParseFilter creates a no-cache [filter.Parser] and parses the given
// expression, failing the test on any error. Accepts [testing.TB] so it can
// be called from both tests and benchmarks.
func MustParseFilter(tb testing.TB, expr string) filter.Node {
	tb.Helper()
	p, err := filter.NewParser(filter.WithParserNoCache())
	if err != nil {
		tb.Fatalf("NewParser() error = %v", err)
	}
	node, err := p.Parse(tb.Context(), expr)
	if err != nil {
		tb.Fatalf("Parse(%q) error = %v", expr, err)
	}
	return node
}
