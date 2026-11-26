// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package testdbname_test

import (
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testdbname"
)

func TestNew(t *testing.T) {
	t.Parallel()

	now := time.Unix(0x6a1b2c3d, 0)
	name := testdbname.New("it_", now)
	require.Regexp(t, regexp.MustCompile(`^it_6a1b2c3d_[0-9a-f]{8}$`), name)
	require.NotEqual(t, name, testdbname.New("it_", now), "the suffix is random")
}

func TestStale(t *testing.T) {
	t.Parallel()

	now := time.Unix(0x6a000000, 0)
	names := map[string]bool{
		testdbname.New("it_", now.Add(-3*time.Hour)): true,
		testdbname.New("it_", now):                   false,
		"it_manual":                                  false,
		"other_6a000000_00000000":                    false,
		"it_zzzzzzzz_00000000":                       false,
	}
	for name, want := range names {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, want, testdbname.Stale(testdbname.Pattern("it_"), name, now.Add(-2*time.Hour)))
		})
	}
}
