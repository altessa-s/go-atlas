// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package testdbname_test

import (
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/internal/testdbname"
)

func BenchmarkStale(b *testing.B) {
	pattern := testdbname.Pattern("it_")
	name := testdbname.New("it_", time.Unix(0x6a000000, 0))
	cutoff := time.Unix(0x6a100000, 0)
	for b.Loop() {
		testdbname.Stale(pattern, name, cutoff)
	}
}
