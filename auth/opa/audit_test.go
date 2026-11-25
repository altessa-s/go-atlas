// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package opa_test

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/audit/decision"
	"github.com/altessa-s/go-atlas/auth/opa"
	"github.com/altessa-s/go-atlas/auth/opa/sources/filesystem"
)

var _ opa.DecisionRecorder = (*decision.Recorder)(nil)

type decisionRecorderFunc func(context.Context, decision.Decision) error

func (f decisionRecorderFunc) Record(ctx context.Context, d decision.Decision) error {
	return f(ctx, d)
}

// errSink is the failure injected by captureSink to exercise FailureRequired.
var errSink = errors.New("sink failure")

// captureSink is a test [decision.Sink] that records every decision and can
// optionally fail every write.
type captureSink struct {
	mu        sync.Mutex
	decisions []decision.Decision
	err       error
}

func (s *captureSink) Record(_ context.Context, d decision.Decision) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.decisions = append(s.decisions, d)
	return s.err
}

func (s *captureSink) all() []decision.Decision {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.decisions)
}

// newAuditEvaluator builds a Manager over the boolean allow policy wired to rec.
func newAuditEvaluator(t *testing.T, rec opa.DecisionRecorder) opa.Evaluator {
	t.Helper()

	tmpDir := t.TempDir()
	writePolicy(t, tmpDir, `
package test.authz
import rego.v1
allow if input.role == "admin"
`)

	writeTestRelease(t, tmpDir)
	source, err := filesystem.New(tmpDir, filesystem.WithReleaseManifest("release.json"))
	require.NoError(t, err)

	manager, err := opa.NewManager(t.Context(), source, "data.test.authz.allow",
		opa.WithAuditRecorder(rec))
	require.NoError(t, err)
	t.Cleanup(func() { _ = manager.Close() })

	return manager.Evaluator()
}

func TestEvaluator_DecisionRecorderPort(t *testing.T) {
	t.Parallel()

	recorderErr := errors.New("recorder failed")
	var got []decision.Decision
	recorder := decisionRecorderFunc(func(_ context.Context, d decision.Decision) error {
		got = append(got, d)
		return recorderErr
	})
	ev := newAuditEvaluator(t, recorder)
	_, err := ev.Evaluate(t.Context(), map[string]any{"role": "admin"})
	require.ErrorIs(t, err, recorderErr)
	require.Len(t, got, 1)
	require.True(t, got[0].Allowed)

	// A missing recorder leaves evaluations unchanged.
	ev = newAuditEvaluator(t, nil)
	res, err := ev.Evaluate(t.Context(), map[string]any{"role": "admin"})
	require.NoError(t, err)
	require.True(t, res.Allow)
}

func TestEvaluator_AuditRecording(t *testing.T) {
	t.Parallel()

	t.Run("RecordsAllow", func(t *testing.T) {
		t.Parallel()

		sink := &captureSink{}
		ev := newAuditEvaluator(t, decision.NewRecorder(sink, decision.WithPolicyMode(decision.PolicyAll)))

		res, err := ev.Evaluate(t.Context(), map[string]any{"role": "admin"})
		require.NoError(t, err)
		require.True(t, res.Allow)

		decisions := sink.all()
		require.Len(t, decisions, 1)
		assert.True(t, decisions[0].Allowed)
		assert.Equal(t, "data.test.authz.allow", decisions[0].Action)
		assert.Empty(t, decisions[0].Reason)
		assert.Equal(t, "opa", decisions[0].Attributes["engine"])
		assert.False(t, decisions[0].Time.IsZero())
	})

	t.Run("RecordsDeny", func(t *testing.T) {
		t.Parallel()

		sink := &captureSink{}
		ev := newAuditEvaluator(t, decision.NewRecorder(sink, decision.WithPolicyMode(decision.PolicyAll)))

		res, err := ev.Evaluate(t.Context(), map[string]any{"role": "user"})
		require.NoError(t, err)
		require.False(t, res.Allow)

		decisions := sink.all()
		require.Len(t, decisions, 1)
		assert.False(t, decisions[0].Allowed)
		assert.Equal(t, "data.test.authz.allow", decisions[0].Action)
		assert.Equal(t, "deny", decisions[0].Reason)
	})

	t.Run("DenyOnlyDefaultDropsAllow", func(t *testing.T) {
		t.Parallel()

		sink := &captureSink{}
		ev := newAuditEvaluator(t, decision.NewRecorder(sink)) // default PolicyDenyOnly

		_, err := ev.Evaluate(t.Context(), map[string]any{"role": "admin"})
		require.NoError(t, err)
		assert.Empty(t, sink.all(), "allow must be dropped under deny-only")

		_, err = ev.Evaluate(t.Context(), map[string]any{"role": "user"})
		require.NoError(t, err)
		require.Len(t, sink.all(), 1, "deny must still be recorded under deny-only")
	})

	t.Run("FailureRequiredOnAllowReturnsError", func(t *testing.T) {
		t.Parallel()

		sink := &captureSink{err: errSink}
		ev := newAuditEvaluator(t, decision.NewRecorder(sink,
			decision.WithPolicyMode(decision.PolicyAll),
			decision.WithFailureMode(decision.FailureRequired)))

		_, err := ev.Evaluate(t.Context(), map[string]any{"role": "admin"})
		require.Error(t, err)
		assert.ErrorIs(t, err, decision.ErrAuditFailed)
	})

	t.Run("FailureRequiredOnDenyIgnoresError", func(t *testing.T) {
		t.Parallel()

		sink := &captureSink{err: errSink}
		ev := newAuditEvaluator(t, decision.NewRecorder(sink,
			decision.WithPolicyMode(decision.PolicyAll),
			decision.WithFailureMode(decision.FailureRequired)))

		res, err := ev.Evaluate(t.Context(), map[string]any{"role": "user"})
		require.NoError(t, err)
		require.False(t, res.Allow)
	})

	t.Run("NoRecorderIsNoOp", func(t *testing.T) {
		t.Parallel()

		ev := newAuditEvaluator(t, nil)

		res, err := ev.Evaluate(t.Context(), map[string]any{"role": "admin"})
		require.NoError(t, err)
		assert.True(t, res.Allow)
	})
}
