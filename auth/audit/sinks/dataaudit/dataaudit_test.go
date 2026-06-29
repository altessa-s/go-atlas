// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package dataaudit_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/audit/sinks/dataaudit"

	authaudit "github.com/altessa-s/go-atlas/auth/audit"
	coreaudit "github.com/altessa-s/go-atlas/data/audit"
)

// stubDispatcher captures submitted events and can simulate a full buffer.
type stubDispatcher struct {
	got  []*coreaudit.Event
	drop bool
}

func (s *stubDispatcher) Submit(e *coreaudit.Event) bool {
	if s.drop {
		return false
	}
	s.got = append(s.got, e)
	return true
}

func (*stubDispatcher) Dropped() int64 { return 0 }

func newAuditor(t *testing.T, disp coreaudit.Dispatcher) *coreaudit.Auditor {
	t.Helper()
	a, err := coreaudit.New(disp)
	require.NoError(t, err)
	require.NoError(t, a.Start())
	return a
}

func TestRecordMapsDeniedDecision(t *testing.T) {
	t.Parallel()

	disp := &stubDispatcher{}
	sink := dataaudit.New(newAuditor(t, disp))

	ts := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	err := sink.Record(t.Context(), authaudit.Decision{
		Time:          ts,
		Allowed:       false,
		Subject:       "u1",
		Action:        "/docs.v1.Docs/Update",
		ResourceType:  "doc",
		ResourceID:    "d1",
		RequiredScope: "docs:write",
		Reason:        "scope_denied",
		Attributes:    map[string]string{"tenant": "t1"},
	})
	require.NoError(t, err)

	require.Len(t, disp.got, 1)
	ev := disp.got[0]
	require.Equal(t, coreaudit.EventTypeAuth, ev.Type)
	require.Equal(t, coreaudit.ResultStatusDenied, ev.Result.Status)
	require.Equal(t, "scope_denied", ev.Result.Message)
	require.Equal(t, "u1", ev.Actor.ID)
	require.Equal(t, "doc", ev.Resource.Type)
	require.Equal(t, "d1", ev.Resource.ID)
	require.Equal(t, "/docs.v1.Docs/Update", ev.Resource.Path)
	require.Equal(t, ts, ev.Timestamp)
	require.Equal(t, "docs:write", ev.Metadata["required_scope"])
	require.Equal(t, "t1", ev.Metadata["tenant"])
}

func TestRecordMapsAllowedDecision(t *testing.T) {
	t.Parallel()

	disp := &stubDispatcher{}
	sink := dataaudit.New(newAuditor(t, disp))

	require.NoError(t, sink.Record(t.Context(), authaudit.Decision{
		Allowed: true,
		Subject: "u1",
		Action:  "act",
	}))
	require.Len(t, disp.got, 1)
	require.Equal(t, coreaudit.ResultStatusSuccess, disp.got[0].Result.Status)
}

func TestRecordDroppedReturnsErrDropped(t *testing.T) {
	t.Parallel()

	sink := dataaudit.New(newAuditor(t, &stubDispatcher{drop: true}))
	err := sink.Record(t.Context(), authaudit.Decision{Action: "act"})
	require.ErrorIs(t, err, dataaudit.ErrDropped)
}

func TestRecordNilAuditorIsNoop(t *testing.T) {
	t.Parallel()
	require.NoError(t, dataaudit.New(nil).Record(t.Context(), authaudit.Decision{Action: "act"}))
}

// TestRecordViaRecorder confirms the bridge satisfies the audit.Sink seam and
// composes with an audit.Recorder end to end.
func TestRecordViaRecorder(t *testing.T) {
	t.Parallel()

	disp := &stubDispatcher{}
	rec := authaudit.NewRecorder(dataaudit.New(newAuditor(t, disp)), authaudit.WithPolicyMode(authaudit.PolicyAll))
	require.NoError(t, rec.Record(t.Context(), authaudit.Decision{Allowed: true, Action: "act"}))
	require.Len(t, disp.got, 1)
}
