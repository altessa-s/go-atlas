// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package base

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMatchFunc_True(t *testing.T) {
	var m Matcher = MatchFunc(func() bool { return true })
	require.True(t, m.Match(), "MatchFunc(true) should return true")
}

func TestMatchFunc_False(t *testing.T) {
	var m Matcher = MatchFunc(func() bool { return false })
	require.False(t, m.Match(), "MatchFunc(false) should return false")
}

func TestMatchFunc_Dynamic(t *testing.T) {
	enabled := false
	m := MatchFunc(func() bool { return enabled })

	require.False(t, m.Match(), "expected false before enabling")
	enabled = true
	require.True(t, m.Match(), "expected true after enabling")
}

func TestMatchFunc_SatisfiesMatcher(t *testing.T) {
	// Compile-time check that MatchFunc satisfies Matcher.
	var _ Matcher = MatchFunc(func() bool { return true })
}
