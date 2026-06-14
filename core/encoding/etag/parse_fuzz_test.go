// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package etag_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/encoding/etag"
)

func FuzzParse(f *testing.F) {
	for _, seed := range []string{`"abc"`, `W/"abc"`, `""`, `W/`, ``, `abc`, "\"a\x80\"", `"a"b"`} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, s string) {
		tag, err := etag.Parse(s)
		if err != nil {
			return
		}
		// An accepted tag must re-parse to itself (String/Parse round-trip).
		again, err := etag.Parse(tag.String())
		require.NoError(t, err)
		require.Equal(t, tag, again)
	})
}
