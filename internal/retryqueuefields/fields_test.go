// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package retryqueuefields_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/retryqueuefields"
)

func TestValidFlag(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		field string
		valid bool
	}{
		{"custom", "statusFlag", true},
		{"whitespace", "  statusFlag ", true},
		{"empty", " ", false},
		{"collision", " reason ", false},
		{"reserved", "nextAttemptAt", false},
		{"operator", "$name", false},
		{"path", "flag.reason", false},
		{"invalid BSON key", "flag\x00reason", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.valid, retryqueuefields.ValidFlag(tc.field, "reason"))
		})
	}
}
