// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package errstatus

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestErrorTypeString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"nil", nil, "<nil>"},
		{"plain", errors.New("x"), "*errors.errorString"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := errorTypeString(tt.err)
			require.Equal(t, tt.want, got)
		})
	}
}
