// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package testhelpers

import (
	"testing"

	"github.com/altessa-s/go-atlas/data/orderby"
)

// MustParseOrderBy creates a no-cache [orderby.Parser] and parses the
// given expression, failing the test on any error. Accepts [testing.TB]
// so it can be called from both tests and benchmarks.
func MustParseOrderBy(tb testing.TB, expr string) orderby.Spec {
	tb.Helper()
	p, err := orderby.NewParser(orderby.WithParserNoCache())
	if err != nil {
		tb.Fatalf("NewParser() error = %v", err)
	}
	ob, err := p.Parse(tb.Context(), expr)
	if err != nil {
		tb.Fatalf("Parse(%q) error = %v", expr, err)
	}
	return ob
}
