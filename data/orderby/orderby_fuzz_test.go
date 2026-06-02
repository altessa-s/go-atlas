// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package orderby_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/orderby"
)

func FuzzParse(f *testing.F) {
	seeds := []string{
		"",
		" ",
		"slug",
		"slug desc",
		"create_time desc, slug",
		"address.city asc, name desc",
		"a,b,c",
		",",
		",a",
		"a,",
		"a,,b",
		"foo bar baz",
		"foo$",
		"foo..bar",
		"1foo",
		"DESC",
		"\t\n",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	p, err := orderby.NewParser(orderby.WithParserNoCache())
	require.NoError(f, err)

	f.Fuzz(func(t *testing.T, in string) {
		// Parse must never panic for any input.
		_, _ = p.Parse(t.Context(), in)
	})
}
