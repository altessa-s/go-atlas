// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package testhelpers

import (
	"testing"

	"github.com/altessa-s/go-atlas/data/filter"
)

// MustParseFilter creates a no-cache [filter.Parser] and parses the given
// expression, failing the test on any error.
func MustParseFilter(t *testing.T, expr string) filter.Node {
	t.Helper()
	p, err := filter.NewParser(filter.WithParserNoCache())
	if err != nil {
		t.Fatalf("NewParser() error = %v", err)
	}
	node, err := p.Parse(t.Context(), expr)
	if err != nil {
		t.Fatalf("Parse(%q) error = %v", expr, err)
	}
	return node
}
