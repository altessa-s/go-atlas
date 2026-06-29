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

	"github.com/altessa-s/go-atlas/auth/audit"
	"github.com/altessa-s/go-atlas/auth/opa"
	"github.com/altessa-s/go-atlas/auth/opa/sources/filesystem"
)

// errSink is the failure injected by captureSink to exercise FailureRequired.
var errSink = errors.New("sink failure")

// captureSink is a test [audit.Sink] that records every decision and can
// optionally fail every write.
type captureSink struct {
	mu        sync.Mutex
	decisions []audit.Decision
	err       error
}

func (s *captureSink) Record(_ context.Context, d audit.Decision) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.decisions = append(s.decisions, d)
	return s.err
}

func (s *captureSink) all() []audit.Decision {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.decisions)
}

// newAuditEvaluator builds a Manager over the boolean allow policy wired to rec.
func newAuditEvaluator(t *testing.T, rec *audit.Recorder) opa.Evaluator {
	t.Helper()

	tmpDir := t.TempDir()
	writePolicy(t, tmpDir, `
package test.authz
import rego.v1
allow if input.role == "admin"
`)

	source, err := filesystem.New(tmpDir)
	require.NoError(t, err)

	manager, err := opa.NewManager(t.Context(), source, "data.test.authz.allow",
		opa.WithAuditRecorder(rec))
	require.NoError(t, err)
	t.Cleanup(func() { _ = manager.Close() })

	return manager.Evaluator()
}

func TestEvaluator_AuditRecording(t *testing.T) {
	t.Parallel()

	t.Run("RecordsAllow", func(t *testing.T) {
		t.Parallel()

		sink := &captureSink{}
		ev := newAuditEvaluator(t, audit.NewRecorder(sink, audit.WithPolicyMode(audit.PolicyAll)))

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
		ev := newAuditEvaluator(t, audit.NewRecorder(sink, audit.WithPolicyMode(audit.PolicyAll)))

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
		ev := newAuditEvaluator(t, audit.NewRecorder(sink)) // default PolicyDenyOnly

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
		ev := newAuditEvaluator(t, audit.NewRecorder(sink,
			audit.WithPolicyMode(audit.PolicyAll),
			audit.WithFailureMode(audit.FailureRequired)))

		_, err := ev.Evaluate(t.Context(), map[string]any{"role": "admin"})
		require.Error(t, err)
		assert.ErrorIs(t, err, audit.ErrAuditFailed)
	})

	t.Run("FailureRequiredOnDenyIgnoresError", func(t *testing.T) {
		t.Parallel()

		sink := &captureSink{err: errSink}
		ev := newAuditEvaluator(t, audit.NewRecorder(sink,
			audit.WithPolicyMode(audit.PolicyAll),
			audit.WithFailureMode(audit.FailureRequired)))

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
