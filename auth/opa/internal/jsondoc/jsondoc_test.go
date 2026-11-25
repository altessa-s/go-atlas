// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package jsondoc_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/opa/internal/jsondoc"
)

func TestDecode(t *testing.T) {
	t.Parallel()
	v, err := jsondoc.Decode([]byte("9007199254740993 \n"))
	require.NoError(t, err)
	require.Equal(t, json.Number("9007199254740993"), v)
	for _, input := range []string{"", "{} {", "[] true", "1 !"} {
		_, err := jsondoc.Decode([]byte(input))
		require.Error(t, err)
	}
}
