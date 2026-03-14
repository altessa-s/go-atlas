// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package base

import "testing"

func TestMatchFunc_True(t *testing.T) {
	var m Matcher = MatchFunc(func() bool { return true })
	if !m.Match() {
		t.Fatal("MatchFunc(true) should return true")
	}
}

func TestMatchFunc_False(t *testing.T) {
	var m Matcher = MatchFunc(func() bool { return false })
	if m.Match() {
		t.Fatal("MatchFunc(false) should return false")
	}
}

func TestMatchFunc_Dynamic(t *testing.T) {
	enabled := false
	m := MatchFunc(func() bool { return enabled })

	if m.Match() {
		t.Fatal("expected false before enabling")
	}
	enabled = true
	if !m.Match() {
		t.Fatal("expected true after enabling")
	}
}

func TestMatchFunc_SatisfiesMatcher(t *testing.T) {
	// Compile-time check that MatchFunc satisfies Matcher.
	var _ Matcher = MatchFunc(func() bool { return true })
}
