// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package decision_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/audit/decision"
)

// captureSink records every decision it receives and can be told to fail.
type captureSink struct {
	got []decision.Decision
	err error
}

func (s *captureSink) Record(_ context.Context, d decision.Decision) error {
	s.got = append(s.got, d)
	return s.err
}

func TestRecordPolicyMode(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		opts    []decision.Option
		allowed bool
		wantLen int
	}{
		{"deny-only records denial", nil, false, 1},
		{"deny-only drops allow", nil, true, 0},
		{"all records denial", []decision.Option{decision.WithPolicyMode(decision.PolicyAll)}, false, 1},
		{"all records allow", []decision.Option{decision.WithPolicyMode(decision.PolicyAll)}, true, 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sink := &captureSink{}
			rec := decision.NewRecorder(sink, tc.opts...)
			require.NoError(t, rec.Record(t.Context(), decision.Decision{Allowed: tc.allowed, Action: "act"}))
			require.Len(t, sink.got, tc.wantLen)
		})
	}
}

func TestRecordFailureMode(t *testing.T) {
	t.Parallel()

	sinkErr := errors.New("sink down")

	t.Run("best-effort swallows sink error", func(t *testing.T) {
		t.Parallel()
		rec := decision.NewRecorder(&captureSink{err: sinkErr}) // default FailureBestEffort
		require.NoError(t, rec.Record(t.Context(), decision.Decision{Action: "act"}))
	})

	t.Run("required propagates wrapping ErrAuditFailed", func(t *testing.T) {
		t.Parallel()
		rec := decision.NewRecorder(&captureSink{err: sinkErr}, decision.WithFailureMode(decision.FailureRequired))
		err := rec.Record(t.Context(), decision.Decision{Action: "act"})
		require.ErrorIs(t, err, decision.ErrAuditFailed)
		require.ErrorIs(t, err, sinkErr)
	})
}

func TestRecordNoopRecorders(t *testing.T) {
	t.Parallel()

	// A nil recorder and a recorder with a nil sink are both valid no-ops.
	var nilRec *decision.Recorder
	require.NoError(t, nilRec.Record(t.Context(), decision.Decision{}))
	require.NoError(t, decision.NewRecorder(nil, decision.WithFailureMode(decision.FailureRequired)).
		Record(t.Context(), decision.Decision{}))
	// A typed nil sink is treated like a nil sink instead of panicking.
	var typedNil *captureSink
	require.NotPanics(t, func() {
		require.NoError(t, decision.NewRecorder(typedNil, decision.WithFailureMode(decision.FailureRequired)).
			Record(t.Context(), decision.Decision{}))
	})
}

func TestRecordPassesDecisionThrough(t *testing.T) {
	t.Parallel()

	sink := &captureSink{}
	rec := decision.NewRecorder(sink, decision.WithPolicyMode(decision.PolicyAll))
	want := decision.Decision{
		Allowed:       true,
		Subject:       "u1",
		Action:        "/docs.v1.Docs/Update",
		ResourceType:  "doc",
		ResourceID:    "d1",
		RequiredScope: "docs:write",
		Reason:        "owner",
		Attributes:    map[string]string{"tenant": "t1"},
	}
	require.NoError(t, rec.Record(t.Context(), want))
	require.Equal(t, []decision.Decision{want}, sink.got)
}

func TestSinkFunc(t *testing.T) {
	t.Parallel()

	var seen decision.Decision
	rec := decision.NewRecorder(decision.SinkFunc(func(_ context.Context, d decision.Decision) error {
		seen = d
		return nil
	}), decision.WithPolicyMode(decision.PolicyAll))
	require.NoError(t, rec.Record(t.Context(), decision.Decision{Action: "act", Allowed: true}))
	require.Equal(t, "act", seen.Action)
}

func TestModeStrings(t *testing.T) {
	t.Parallel()
	require.Equal(t, "deny_only", decision.PolicyDenyOnly.String())
	require.Equal(t, "all", decision.PolicyAll.String())
	require.Equal(t, "best_effort", decision.FailureBestEffort.String())
	require.Equal(t, "required", decision.FailureRequired.String())
}
