// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package opa_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-core/runtime/cycle"
	"github.com/altessa-s/go-atlas/auth/opa"
	"github.com/altessa-s/go-atlas/auth/opa/sources/embed"
	"github.com/altessa-s/go-atlas/observability/health"

	schedtask "github.com/altessa-s/go-horai/task"
)

// The admin set is baked into the module so these tests vary modules;
// TestManager_BundleDataLoadsAndReloads covers bundles carrying Data.
const policyTemplate = "package t\nimport rego.v1\nallow if input.user == %q\n"

// mutableSource serves a swappable bundle and an optional fetch error.
type mutableSource struct {
	mu     sync.Mutex
	bundle *opa.PolicyBundle
	err    error
}

func (s *mutableSource) Name() string { return "mutable" }

func (s *mutableSource) Fetch(context.Context) (*opa.PolicyBundle, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.bundle, s.err
}

func (s *mutableSource) Close() error { return nil }

func (s *mutableSource) set(b *opa.PolicyBundle, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bundle, s.err = b, err
}

func adminsBundle(admin string) *opa.PolicyBundle {
	return opa.NewPolicyBundle(map[string][]byte{"p.rego": fmt.Appendf(nil, policyTemplate, admin)})
}

// recordingRegistrar captures registered tasks.
type recordingRegistrar struct {
	mu    sync.Mutex
	tasks []schedtask.Config
	err   error
}

func (r *recordingRegistrar) Register(_ context.Context, cfg schedtask.Config) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return r.err
	}
	r.tasks = append(r.tasks, cfg)
	return nil
}

type recordingHealth struct {
	mu    sync.Mutex
	names []string
}

func (h *recordingHealth) RegisterService(name string, _ health.Checker) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.names = append(h.names, name)
}

func TestPolicyBundle_Accessors(t *testing.T) {
	t.Parallel()

	empty := opa.NewPolicyBundle(nil)
	require.True(t, empty.IsEmpty())
	require.Empty(t, slices.Collect(empty.DataKeysIter()))
	for range empty.DataIter() {
		require.Fail(t, "an empty bundle has no data")
	}

	b, err := opa.NewPolicyBundleWithData(
		map[string][]byte{"a.rego": []byte("package a"), "b.rego": []byte("package b")},
		map[string]any{"x": 1},
	)
	require.NoError(t, err)
	require.False(t, b.IsEmpty())
	require.Equal(t, 2, b.ModuleCount())
	keys := slices.Sorted(b.ModuleKeysIter())
	require.Equal(t, []string{"a.rego", "b.rego"}, keys)
	mods := map[string]string{}
	for k, v := range b.ModulesIter() {
		mods[k] = string(v)
	}
	require.Equal(t, "package a", mods["a.rego"])
	require.Equal(t, []string{"x"}, slices.Collect(b.DataKeysIter()))
	for k, v := range b.DataIter() {
		require.Equal(t, "x", k)
		require.Equal(t, 1, v)
	}

	// Data participates in the revision: same modules, different data.
	other, err := opa.NewPolicyBundleWithData(b.Modules, map[string]any{"x": 2})
	require.NoError(t, err)
	require.NotEqual(t, b.Revision, other.Revision)
	same, err := opa.NewPolicyBundleWithData(b.Modules, map[string]any{"x": 1})
	require.NoError(t, err)
	require.Equal(t, b.Revision, same.Revision)

}

func TestEnumStrings(t *testing.T) {
	t.Parallel()
	require.Equal(t, "drop_newest", opa.OverflowPolicyDropNewest.String())
	require.Equal(t, "drop_oldest", opa.OverflowPolicyDropOldest.String())
	require.Equal(t, "block", opa.OverflowPolicyBlock.String())
	require.Equal(t, "unknown", opa.BufferOverflowPolicy(99).String())
	require.Equal(t, "policy_updated", opa.EventTypePolicyUpdated.String())
	require.Equal(t, "policy_error", opa.EventTypePolicyError.String())
	require.Equal(t, "unknown", opa.EventType(99).String())
}

func TestManager_ReloadSwapsDecisions(t *testing.T) {
	t.Parallel()
	src := &mutableSource{bundle: adminsBundle("alice")}
	hc := &recordingHealth{}
	m, err := opa.NewManager(t.Context(), src, "data.t.allow", opa.WithHealthCoordinator(hc))
	require.NoError(t, err)
	t.Cleanup(func() { _ = m.Close() })

	require.Equal(t, "data.t.allow", m.Query())
	require.Equal(t, "data.t.allow", m.Evaluator().Query())
	require.Same(t, src, m.Source().(*mutableSource))
	require.EqualValues(t, 1, m.ModuleCount())
	require.Equal(t, []string{"opa"}, hc.names)

	res, err := m.Evaluator().Evaluate(t.Context(), map[string]any{"user": "alice"})
	require.NoError(t, err)
	require.True(t, res.Allow)
	res, err = m.Evaluator().Evaluate(t.Context(), map[string]any{"user": "bob"})
	require.NoError(t, err)
	require.False(t, res.Allow)

	// A new bundle replaces the old policy.
	src.set(adminsBundle("bob"), nil)
	require.NoError(t, m.RunUpdateCycle(t.Context()))
	res, err = m.Evaluator().Evaluate(t.Context(), map[string]any{"user": "bob"})
	require.NoError(t, err)
	require.True(t, res.Allow)
	res, err = m.Evaluator().Evaluate(t.Context(), map[string]any{"user": "alice"})
	require.NoError(t, err)
	require.False(t, res.Allow)
}

func TestManager_SchedulerRegistration(t *testing.T) {
	t.Parallel()
	src := &mutableSource{bundle: adminsBundle("alice")}
	reg := &recordingRegistrar{}
	m, err := opa.NewManager(t.Context(), src, "data.t.allow",
		opa.WithScheduler(reg), opa.WithSchedulerTaskID("profile-policy-refresh"), opa.WithUpdateSchedule("*/5 * * * * *", true))
	require.NoError(t, err)
	t.Cleanup(func() { _ = m.Close() })

	require.Len(t, reg.tasks, 1)
	task := reg.tasks[0]
	require.Equal(t, "profile-policy-refresh", task.ID)
	require.Equal(t, "*/5 * * * * *", task.Schedule)
	require.True(t, task.RunOnStart)

	require.ErrorIs(t, m.RunUpdateCycle(t.Context()), cycle.ErrManaged)

	src.set(adminsBundle("bob"), nil)
	require.NoError(t, task.Func(t.Context()))
	res, err := m.Evaluator().Evaluate(t.Context(), map[string]any{"user": "bob"})
	require.NoError(t, err)
	require.True(t, res.Allow, "the scheduled task must reload policies")
}

func TestManager_DefaultSchedulerTaskID(t *testing.T) {
	t.Parallel()
	policy := &fstest.MapFile{Data: fmt.Appendf(nil, policyTemplate, "alice")}
	files := fstest.MapFS{"team-a/p.rego": policy, "team-b/p.rego": policy}
	register := func(dir, query string, opts ...opa.Option) string {
		t.Helper()
		src, err := embed.New(files, dir)
		require.NoError(t, err)
		reg := &recordingRegistrar{}
		opts = append(opts, opa.WithScheduler(reg), opa.WithUpdateSchedule("@every 1m", false))
		m, err := opa.NewManager(t.Context(), src, query, opts...)
		require.NoError(t, err)
		require.NoError(t, m.Close())
		require.Len(t, reg.tasks, 1)
		return reg.tasks[0].ID
	}

	const query = "data.t.allow"
	id := register("team-a", query)
	for range 3 {
		require.Equal(t, id, register("team-a", query))
	}
	require.Equal(t, "opa-policy-update-26316125d6f7a3a9293d42d9", id)
	require.Equal(t, id, register("team-a", query, opa.WithSchedulerTaskID("")))
	require.NotEqual(t, id, register("team-b", query))
	require.NotEqual(t, id, register("team-a", "data.t"))
}

func TestManager_SchedulerWithoutScheduleRegistersNothing(t *testing.T) {
	t.Parallel()
	reg := &recordingRegistrar{}
	m, err := opa.NewManager(t.Context(), &mutableSource{bundle: adminsBundle("a")}, "data.t.allow", opa.WithScheduler(reg))
	require.NoError(t, err)
	t.Cleanup(func() { _ = m.Close() })
	require.Empty(t, reg.tasks)
	require.NoError(t, m.RunUpdateCycle(t.Context()))
}

func TestManager_SchedulerRegistrationError(t *testing.T) {
	t.Parallel()
	errBoom := errors.New("boom")
	reg := &recordingRegistrar{err: errBoom}
	_, err := opa.NewManager(t.Context(), &mutableSource{bundle: adminsBundle("a")}, "data.t.allow",
		opa.WithScheduler(reg), opa.WithUpdateSchedule("@every 1m", false))
	require.ErrorIs(t, err, errBoom)
}

func TestManager_ErrorEventsAndHealth(t *testing.T) {
	t.Parallel()
	src := &mutableSource{bundle: adminsBundle("a")}
	m, err := opa.NewManager(t.Context(), src, "data.t.allow")
	require.NoError(t, err)
	t.Cleanup(func() { _ = m.Close() })
	require.Equal(t, health.StatusServing, m.CheckHealth(t.Context()))

	w, err := m.Watch(t.Context(), opa.WatchOptions{EventTypes: []opa.EventType{opa.EventTypePolicyError}})
	require.NoError(t, err)
	t.Cleanup(w.Stop)

	errFetch := errors.New("fetch failed")
	src.set(nil, errFetch)
	require.Error(t, m.RunUpdateCycle(t.Context()))

	select {
	case ev := <-w.Events:
		require.Equal(t, opa.EventTypePolicyError, ev.Type)
		require.ErrorIs(t, ev.Error, errFetch)
	case <-time.After(5 * time.Second):
		require.Fail(t, "no error event delivered")
	}
	require.Equal(t, health.StatusNotServing, m.CheckHealth(t.Context()))

	src.set(adminsBundle("a"), nil)
	require.NoError(t, m.RunUpdateCycle(t.Context()))
	require.Equal(t, health.StatusServing, m.CheckHealth(t.Context()))

	// A successful update must not reach an error-only subscriber.
	src.set(adminsBundle("b"), nil)
	require.NoError(t, m.RunUpdateCycle(t.Context()))
	select {
	case ev := <-w.Events:
		require.Failf(t, "unexpected event", "%v", ev.Type)
	default:
	}
	require.Equal(t, health.StatusServing, m.CheckHealth(t.Context()))

	require.NoError(t, m.Close())
	require.Equal(t, health.StatusNotServing, m.CheckHealth(t.Context()))
	_, err = m.Watch(t.Context(), opa.WatchOptions{})
	require.ErrorIs(t, err, opa.ErrManagerClosed)
	select {
	case <-w.Done:
	case <-time.After(5 * time.Second):
		require.Fail(t, "Close must end subscriptions")
	}
}

// update pushes n distinct revisions through the manager.
func update(t *testing.T, m *opa.Manager, src *mutableSource, admins ...string) []string {
	t.Helper()
	revs := make([]string, 0, len(admins))
	for _, a := range admins {
		b := adminsBundle(a)
		src.set(b, nil)
		require.NoError(t, m.RunUpdateCycle(t.Context()))
		revs = append(revs, b.Revision)
	}
	return revs
}

func TestManager_WatchOverflowPolicies(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		policy opa.BufferOverflowPolicy
		want   func(revs []string) string
	}{
		{name: "drop newest keeps first", policy: opa.OverflowPolicyDropNewest, want: func(r []string) string { return r[0] }},
		{name: "drop oldest keeps last", policy: opa.OverflowPolicyDropOldest, want: func(r []string) string { return r[len(r)-1] }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			src := &mutableSource{bundle: adminsBundle("seed")}
			m, err := opa.NewManager(t.Context(), src, "data.t.allow")
			require.NoError(t, err)
			t.Cleanup(func() { _ = m.Close() })

			w, err := m.Watch(t.Context(), opa.WatchOptions{BufferSize: 1, BufferOverflowPolicy: tc.policy})
			require.NoError(t, err)
			t.Cleanup(w.Stop)

			revs := update(t, m, src, "a", "b", "c")
			ev := <-w.Events
			require.Equal(t, tc.want(revs), ev.Revision)
			select {
			case extra := <-w.Events:
				require.Failf(t, "buffer must hold one event", "%v", extra.Revision)
			default:
			}
		})
	}
}

func TestManager_WatchBlockPolicyDeliversEverything(t *testing.T) {
	t.Parallel()
	src := &mutableSource{bundle: adminsBundle("seed")}
	m, err := opa.NewManager(t.Context(), src, "data.t.allow")
	require.NoError(t, err)
	t.Cleanup(func() { _ = m.Close() })

	w, err := m.Watch(t.Context(), opa.WatchOptions{BufferSize: 1, BufferOverflowPolicy: opa.OverflowPolicyBlock})
	require.NoError(t, err)

	got := make(chan []string, 1)
	go func() {
		var revs []string
		for ev := range w.Events {
			revs = append(revs, ev.Revision)
			if len(revs) == 3 {
				break
			}
		}
		got <- revs
	}()

	revs := update(t, m, src, "a", "b", "c")
	select {
	case r := <-got:
		require.Equal(t, revs, r)
	case <-time.After(5 * time.Second):
		require.Fail(t, "blocking subscriber must receive every event")
	}
	w.Stop()
	w.Stop() // idempotent
}

// TestManager_BundleDataLoadsAndReloads pins that a bundle carrying Data is
// written to the store and evaluated against, on the initial load and on a
// reload, without the store panicking on a stale transaction.
func TestManager_BundleDataLoadsAndReloads(t *testing.T) {
	t.Parallel()
	const module = "package t\nimport rego.v1\nallow if input.user in data.admins\n"
	dataBundle := func(admins ...string) *opa.PolicyBundle {
		b, err := opa.NewPolicyBundleWithData(
			map[string][]byte{"p.rego": []byte(module)},
			map[string]any{"admins": admins},
		)
		require.NoError(t, err)
		return b
	}
	src := &mutableSource{bundle: dataBundle("alice")}
	m, err := opa.NewManager(t.Context(), src, "data.t.allow")
	require.NoError(t, err)
	t.Cleanup(func() { _ = m.Close() })

	res, err := m.Evaluator().Evaluate(t.Context(), map[string]any{"user": "alice"})
	require.NoError(t, err)
	require.True(t, res.Allow)

	src.set(dataBundle("bob"), nil)
	require.NoError(t, m.RunUpdateCycle(t.Context()))
	res, err = m.Evaluator().Evaluate(t.Context(), map[string]any{"user": "bob"})
	require.NoError(t, err)
	require.True(t, res.Allow)
	res, err = m.Evaluator().Evaluate(t.Context(), map[string]any{"user": "alice"})
	require.NoError(t, err)
	require.False(t, res.Allow)
}

func TestRevisionFraming(t *testing.T) {
	t.Parallel()
	a, err := opa.NewPolicyBundleWithData(nil, map[string]any{"quota": 12})
	require.NoError(t, err)
	b, err := opa.NewPolicyBundleWithData(nil, map[string]any{"quota1": 2})
	require.NoError(t, err)
	require.NotEqual(t, a.Revision, b.Revision)
	require.NotEqual(t, a.Revision, opa.NewPolicyBundle(map[string][]byte{"quota": []byte("12")}).Revision)
}

func TestRevisionMarshalError(t *testing.T) {
	t.Parallel()
	active := adminsBundle("alice")
	src := &mutableSource{bundle: active}
	m, err := opa.NewManager(t.Context(), src, "data.t.allow")
	require.NoError(t, err)
	t.Cleanup(func() { _ = m.Close() })
	b, err := opa.NewPolicyBundleWithData(active.Modules, map[string]any{"ch": make(chan int)})
	var unsupported *json.UnsupportedTypeError
	require.ErrorAs(t, err, &unsupported)
	require.Nil(t, b)
	src.set(b, err)
	require.ErrorIs(t, m.RunUpdateCycle(t.Context()), opa.ErrBundleFetchFailed)
	require.Equal(t, active.Revision, m.Revision())

}

func TestDocumentTreeReload(t *testing.T) {
	t.Parallel()
	src := &mutableSource{bundle: adminsBundle("seed")}
	m, err := opa.NewManager(t.Context(), src, "data.roles.admin")
	require.NoError(t, err)
	t.Cleanup(func() { _ = m.Close() })
	for _, docs := range []map[string]any{
		{"roles/admin": true, "roles": map[string]any{"user": true}},
		{"roles": map[string]any{"user": false}, "roles/admin": true},
	} {
		b, err := opa.NewPolicyBundleWithData(src.bundle.Modules, docs)
		require.NoError(t, err)
		src.set(b, nil)
		require.NoError(t, m.RunUpdateCycle(t.Context()))
	}
	active := m.Revision()
	for _, parent := range []any{false, nil, []any{}, map[string]any{"admin": false}} {
		b, err := opa.NewPolicyBundleWithData(src.bundle.Modules, map[string]any{"roles": parent, "roles/admin": false})
		require.NoError(t, err)
		src.set(b, nil)
		require.ErrorIs(t, m.RunUpdateCycle(t.Context()), opa.ErrInvalidDataPath)
		require.Equal(t, active, m.Revision())
	}
	res, err := m.Evaluator().Evaluate(t.Context(), nil)
	require.NoError(t, err)
	require.True(t, res.Allow)
}

func TestInitialSnapshotEmptyRevision(t *testing.T) {
	t.Parallel()
	b := adminsBundle("alice")
	b.Revision = ""
	m, err := opa.NewManager(t.Context(), &mutableSource{bundle: b}, "data.t.allow")
	require.NoError(t, err)
	defer m.Close()
	result, err := m.Evaluator().Evaluate(t.Context(), map[string]any{"user": "alice"})
	require.NoError(t, err)
	require.True(t, result.Allow)
}

func TestManager_ScheduleRequiresScheduler(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"missing", "typed nil"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			source := &mutableSource{bundle: adminsBundle("alice")}
			opts := []opa.Option{opa.WithUpdateSchedule("@every 1m", false)}
			if name == "typed nil" {
				opts = append(opts, opa.WithScheduler((*recordingRegistrar)(nil)))
			}
			_, err := opa.NewManager(t.Context(), source, "data.t.allow", opts...)
			require.ErrorIs(t, err, opa.ErrSchedulerRequired)
		})
	}
}

func TestManager_StaticWithoutScheduler(t *testing.T) {
	t.Parallel()

	source := &mutableSource{bundle: adminsBundle("alice")}
	manager, err := opa.NewManager(t.Context(), source, "data.t.allow")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, manager.Close()) })
	require.NoError(t, manager.RunUpdateCycle(t.Context()))
}
