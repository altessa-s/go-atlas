// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package storagetest

import (
	"testing"

	"github.com/altessa-s/go-atlas/service/scheduler"
)

// contracts lists every storage contract of this package; [Run] executes each.
var contracts = []struct {
	name  string
	check func(*testing.T, scheduler.Storage)
}{
	{"FinishRun", FinishRun},
	{"ReplaceTaskIf", ReplaceTaskIf},
	{"CreateTask", CreateTask},
	{"RenewRun", RenewRun},
	{"ClaimRunRequiresFinishedRun", ClaimRunRequiresFinishedRun},
	{"ClaimRunFencesOccurrence", ClaimRunFencesOccurrence},
	{"RunIDRoundTrip", RunIDRoundTrip},
}

// Run executes every storage contract as a parallel subtest named after it, each
// on a fresh store from newStore. A backend wires the whole suite with one call,
// so a new contract cannot be left out of a single backend.
func Run(t *testing.T, newStore func(testing.TB) scheduler.Storage) {
	t.Helper()
	for _, c := range contracts {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			c.check(t, newStore(t))
		})
	}
}
