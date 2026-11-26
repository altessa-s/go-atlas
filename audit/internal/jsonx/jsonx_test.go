// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package jsonx_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/audit/internal/jsonx"
)

func TestDecode(t *testing.T) {
	t.Parallel()

	t.Run("preserves_large_integer_and_trailing_whitespace", func(t *testing.T) {
		t.Parallel()
		var got map[string]any
		require.NoError(t, jsonx.Decode([]byte("{\"seq\":9007199254740993}\n\t "), &got))
		require.Equal(t, json.Number("9007199254740993"), got["seq"])
	})

	for _, tc := range []struct {
		name  string
		input string
	}{
		{name: "empty", input: ""},
		{name: "truncated", input: "{"},
		{name: "malformed", input: "{not json"},
		{name: "trailing_object", input: `{"a":1}{"b":2}`},
		{name: "trailing_number", input: `{"a":1} 2`},
		{name: "trailing_brace", input: `{"a":1}}`},
		{name: "trailing_comma", input: `{"a":1},`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var got map[string]any
			err := jsonx.Decode([]byte(tc.input), &got)
			require.Error(t, err)
			_, ok := errors.AsType[*json.SyntaxError](err)
			require.True(t, ok, "expected *json.SyntaxError, got %T: %v", err, err)
		})
	}
}
