// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package audit_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/audit"
)

// captureSink records every decision it receives and can be told to fail.
type captureSink struct {
	got []audit.Decision
	err error
}

func (s *captureSink) Record(_ context.Context, d audit.Decision) error {
	s.got = append(s.got, d)
	return s.err
}

func TestRecordPolicyMode(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		opts    []audit.Option
		allowed bool
		wantLen int
	}{
		{"deny-only records denial", nil, false, 1},
		{"deny-only drops allow", nil, true, 0},
		{"all records denial", []audit.Option{audit.WithPolicyMode(audit.PolicyAll)}, false, 1},
		{"all records allow", []audit.Option{audit.WithPolicyMode(audit.PolicyAll)}, true, 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sink := &captureSink{}
			rec := audit.NewRecorder(sink, tc.opts...)
			require.NoError(t, rec.Record(t.Context(), audit.Decision{Allowed: tc.allowed, Action: "act"}))
			require.Len(t, sink.got, tc.wantLen)
		})
	}
}

func TestRecordFailureMode(t *testing.T) {
	t.Parallel()

	sinkErr := errors.New("sink down")

	t.Run("best-effort swallows sink error", func(t *testing.T) {
		t.Parallel()
		rec := audit.NewRecorder(&captureSink{err: sinkErr}) // default FailureBestEffort
		require.NoError(t, rec.Record(t.Context(), audit.Decision{Action: "act"}))
	})

	t.Run("required propagates wrapping ErrAuditFailed", func(t *testing.T) {
		t.Parallel()
		rec := audit.NewRecorder(&captureSink{err: sinkErr}, audit.WithFailureMode(audit.FailureRequired))
		err := rec.Record(t.Context(), audit.Decision{Action: "act"})
		require.ErrorIs(t, err, audit.ErrAuditFailed)
		require.ErrorIs(t, err, sinkErr)
	})
}

func TestRecordNoopRecorders(t *testing.T) {
	t.Parallel()

	// A nil recorder and a recorder with a nil sink are both valid no-ops.
	var nilRec *audit.Recorder
	require.NoError(t, nilRec.Record(t.Context(), audit.Decision{}))
	require.NoError(t, audit.NewRecorder(nil, audit.WithFailureMode(audit.FailureRequired)).
		Record(t.Context(), audit.Decision{}))
}

func TestRecordPassesDecisionThrough(t *testing.T) {
	t.Parallel()

	sink := &captureSink{}
	rec := audit.NewRecorder(sink, audit.WithPolicyMode(audit.PolicyAll))
	want := audit.Decision{
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
	require.Equal(t, []audit.Decision{want}, sink.got)
}

func TestSinkFunc(t *testing.T) {
	t.Parallel()

	var seen audit.Decision
	rec := audit.NewRecorder(audit.SinkFunc(func(_ context.Context, d audit.Decision) error {
		seen = d
		return nil
	}), audit.WithPolicyMode(audit.PolicyAll))
	require.NoError(t, rec.Record(t.Context(), audit.Decision{Action: "act", Allowed: true}))
	require.Equal(t, "act", seen.Action)
}

func TestModeStrings(t *testing.T) {
	t.Parallel()
	require.Equal(t, "deny_only", audit.PolicyDenyOnly.String())
	require.Equal(t, "all", audit.PolicyAll.String())
	require.Equal(t, "best_effort", audit.FailureBestEffort.String())
	require.Equal(t, "required", audit.FailureRequired.String())
}
