// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package saga_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/saga"
)

// allStatuses is the closed set of saga states, transcribed from the package
// rather than derived from it: an oracle that enumerates by reflection would
// silently accept a new state nobody classified.
var allStatuses = []saga.Status{
	saga.StatusRunning,
	saga.StatusCompensating,
	saga.StatusCompleted,
	saga.StatusCompensated,
	saga.StatusFailed,
}

// FuzzOnlyKnownStatusesAreTerminal pins the classification an orchestrator
// resumes on.
//
// IsTerminal is what decides whether a recovered instance is picked up and run
// or left alone. A state wrongly called terminal is a saga abandoned mid-flight
// with its compensations unrun; one wrongly called live is a completed saga
// replayed on every recovery sweep. An unknown string — a state written by a
// newer version, or corrupted in the store — must fall on the safe side, which
// here is "not terminal, so a human or a retry looks at it".
func FuzzOnlyKnownStatusesAreTerminal(f *testing.F) {
	for _, status := range allStatuses {
		f.Add(string(status))
	}
	f.Add("")
	f.Add("completed")  // Wrong case.
	f.Add("COMPLETED ") // Trailing space.
	f.Add("UNKNOWN")

	f.Fuzz(func(t *testing.T, raw string) {
		status := saga.Status(raw)

		terminal := status.IsTerminal()
		if terminal {
			require.Contains(t, allStatuses, status,
				"an unrecognized status was treated as terminal, so its saga will never resume: %q", raw)
		}

		switch status {
		case saga.StatusCompleted, saga.StatusCompensated, saga.StatusFailed:
			require.True(t, terminal, "%q is a final state but was reported live", raw)
		case saga.StatusRunning, saga.StatusCompensating:
			require.False(t, terminal, "%q is still working but was reported terminal", raw)
		}
	})
}

// FuzzInstanceCloneIsIndependent pins that a clone shares no mutable state with
// its original.
//
// Clone exists so a recovery pass can work on a snapshot while the live
// instance keeps running. A shallow copy that shared the step slice would let
// one of them observe the other's half-written steps — the classic source of a
// saga that compensates a stage it never completed.
func FuzzInstanceCloneIsIndependent(f *testing.F) {
	f.Add("saga-1", "step-a", uint8(3), 2)
	f.Add("", "", uint8(0), 0)
	f.Add("saga-2", "step-b", uint8(1), 5)

	f.Fuzz(func(t *testing.T, id, stepName string, statusSel uint8, steps int) {
		if steps < 0 || steps > 64 {
			t.Skip("an implausible step count says nothing about Clone")
		}

		original := &saga.Instance{
			ID:        id,
			Status:    allStatuses[int(statusSel)%len(allStatuses)],
			UpdatedAt: time.Unix(0, 0).UTC(),
		}
		for i := 0; i < steps; i++ {
			original.Steps = append(original.Steps, saga.StepRecord{Name: stepName})
		}

		clone := original.Clone()
		require.NotNil(t, clone)

		before, err := json.Marshal(clone)
		require.NoError(t, err)

		// Mutate the original in every way a running saga would.
		original.Status = saga.StatusFailed
		original.ID += "-mutated"
		for i := range original.Steps {
			original.Steps[i].Name += "-mutated"
		}
		original.Steps = append(original.Steps, saga.StepRecord{Name: "appended"})

		after, err := json.Marshal(clone)
		require.NoError(t, err)

		require.JSONEq(t, string(before), string(after),
			"mutating the original changed its clone, so the two share state")
	})
}
