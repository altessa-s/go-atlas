// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package postgres

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTextArray(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		src  any
		want []string
	}{
		"plain elements":     {src: `{alpha,beta}`, want: []string{"alpha", "beta"}},
		"quoted and escaped": {src: `{"two words","quo\"te","back\\slash","com,ma"}`, want: []string{"two words", `quo"te`, `back\slash`, "com,ma"}},
		"non-ASCII":          {src: []byte(`{Нейросеть,"ИИ модель"}`), want: []string{"Нейросеть", "ИИ модель"}},
		"empty array":        {src: `{}`, want: []string{}},
		"null array":         {src: nil, want: nil},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := []string{"stale"}
			require.NoError(t, TextArray(&got).Scan(tc.src))
			require.Equal(t, tc.want, got)
		})
	}

	var got []string
	require.Error(t, TextArray(&got).Scan(`{alpha,NULL}`))
	require.Error(t, TextArray(&got).Scan(`{unterminated`))
}
