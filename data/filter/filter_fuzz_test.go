// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filter_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/data/filter"
)

func FuzzParse(f *testing.F) {
	f.Add(`name == "John"`)
	f.Add(`age > 18 && active == true`)
	f.Add(`status in ["a", "b"]`)
	f.Add(`name.contains("oh")`)
	f.Add(``)
	f.Add(`((((`)
	f.Add(`== ==`)

	p, err := filter.NewParser(filter.WithParserNoCache())
	if err != nil {
		f.Fatal(err)
	}

	f.Fuzz(func(t *testing.T, input string) {
		// Should not panic regardless of input.
		_, _ = p.Parse(input)
	})
}
