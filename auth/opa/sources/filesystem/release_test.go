// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filesystem

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/opa"
	"github.com/altessa-s/go-atlas/observability/health"

	corehash "github.com/altessa-s/go-atlas-core/encoding/hash"
	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

// The filesystem read seam lets the test replace a release between reads.
type releaseSource struct {
	*Source
	readFile func(string) ([]byte, error)
}

func (s releaseSource) Fetch(_ context.Context) (*opa.PolicyBundle, error) {
	return s.fetch(s.readFile)
}

func TestFilesystemRelease_RolloutBetweenReads(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	modulePath := filepath.Join(dir, "a.rego")
	dataPath := filepath.Join(dir, "z.json")
	manifestPath := filepath.Join(dir, "release.json")
	oldModule := []byte(`package policy
allow := data.z == "old"`)
	newModule := []byte(`package policy
allow := data.z != "new"`)
	oldData, newData := []byte(`"old"`), []byte(`"new"`)
	publish := func(module, data []byte) {
		payload, err := json.Marshal(map[string]string{
			"a.rego": corehash.SHA256HexBytes(module),
			"z.json": corehash.SHA256HexBytes(data),
		})
		require.NoError(t, err)
		staged := filepath.Join(dir, ".release.tmp")
		require.NoError(t, os.WriteFile(staged, payload, 0o600))
		require.NoError(t, os.Rename(staged, manifestPath))
	}
	require.NoError(t, os.WriteFile(modulePath, oldModule, 0o600))
	require.NoError(t, os.WriteFile(dataPath, oldData, 0o600))
	publish(oldModule, oldData)

	source, err := New(dir, WithReleaseManifest("release.json"), WithIncludeData())
	require.NoError(t, err)

	var afterModule func()
	var reads []string
	manager, err := opa.NewManager(t.Context(), releaseSource{source, func(name string) ([]byte, error) {
		reads = append(reads, filepath.Base(name))
		content, err := os.ReadFile(name)
		if name == modulePath && afterModule != nil {
			afterModule()
		}
		return content, err
	}}, "data.policy.allow")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, manager.Close()) })

	check := func(allow bool, status health.ServingStatus) {
		t.Helper()
		result, err := manager.Evaluator().Evaluate(t.Context(), nil)
		require.NoError(t, err)
		require.Equal(t, allow, result.Allow)
		require.Equal(t, status, manager.CheckHealth(t.Context()))
	}

	reads = nil
	afterModule = func() {
		require.NoError(t, os.WriteFile(modulePath, newModule, 0o600))
		require.NoError(t, os.WriteFile(dataPath, newData, 0o600))
		publish(newModule, newData)
	}
	require.ErrorIs(t, manager.RunUpdateCycle(t.Context()), ErrChecksumMismatch)
	require.Equal(t, []string{"release.json", "a.rego", "z.json"}, reads)
	check(true, health.StatusNotServing)

	afterModule = nil
	require.NoError(t, os.Remove(dataPath))
	require.ErrorIs(t, manager.RunUpdateCycle(t.Context()), ErrMissingPolicyFile)
	check(true, health.StatusNotServing)

	require.NoError(t, os.WriteFile(dataPath, newData, 0o600))
	require.NoError(t, manager.RunUpdateCycle(t.Context()))
	check(false, health.StatusServing)
}

func TestFilesystemRelease_AtomicRootRollout(t *testing.T) {
	t.Parallel()

	const oldRoot, newRoot = "releases/1", "releases/2"
	dir := t.TempDir()
	oldModule := []byte("package policy\nallow := data.z == \"old\"")
	newModule := []byte("package policy\nallow := data.z != \"new\"")
	oldData, newData := []byte(`"old"`), []byte(`"new"`)
	for _, root := range []string{oldRoot, newRoot} {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, root), 0o700))
	}
	write := func(root, name string, content []byte) {
		t.Helper()
		require.NoError(t, os.WriteFile(filepath.Join(dir, root, name), content, 0o600))
	}
	entries := func(module, data []byte) map[string]string {
		return map[string]string{"a.rego": corehash.SHA256HexBytes(module), "z.json": corehash.SHA256HexBytes(data)}
	}
	write(oldRoot, "a.rego", oldModule)
	write(oldRoot, "z.json", oldData)
	publishFilesystemManifest(t, dir, map[string]any{"root": oldRoot, "files": entries(oldModule, oldData)})

	source, err := New(dir, WithReleaseManifest("release.json"), WithIncludeData())
	require.NoError(t, err)
	bundle, err := source.Fetch(t.Context())
	require.NoError(t, err)
	require.Equal(t, map[string]any{"z": "old"}, bundle.Data)
	var afterRead func(string)
	var reads []string
	manager, err := opa.NewManager(t.Context(), releaseSource{source, func(name string) ([]byte, error) {
		rel, err := filepath.Rel(dir, name)
		require.NoError(t, err)
		reads = append(reads, filepath.ToSlash(rel))
		content, err := os.ReadFile(name)
		if afterRead != nil {
			afterRead(filepath.ToSlash(rel))
		}
		return content, err
	}}, "data.policy.allow")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, manager.Close()) })
	watch, err := manager.Watch(t.Context(), opa.DefaultWatchOptions())
	require.NoError(t, err)
	t.Cleanup(watch.Stop)
	oldRevision := manager.Revision()
	checkServing := func(revision string, allow bool) {
		t.Helper()
		require.Equal(t, health.StatusServing, manager.CheckHealth(t.Context()))
		require.Equal(t, revision, manager.Revision())
		result, err := manager.Evaluator().Evaluate(t.Context(), nil)
		require.NoError(t, err)
		require.Equal(t, allow, result.Allow)
	}

	write(newRoot, "a.rego", newModule)
	require.NoError(t, manager.RunUpdateCycle(t.Context()))
	checkServing(oldRevision, true)
	write(newRoot, "z.json", newData)
	reads = nil
	afterRead = func(name string) {
		if name == oldRoot+"/a.rego" {
			publishFilesystemManifest(t, dir, map[string]any{"root": newRoot, "files": entries(newModule, newData)})
			checkServing(oldRevision, true)
		}
	}
	require.NoError(t, manager.RunUpdateCycle(t.Context()))
	require.Equal(t, []string{"release.json", oldRoot + "/a.rego", oldRoot + "/z.json"}, reads)
	checkServing(oldRevision, true)
	require.Empty(t, watch.Events)

	require.NoError(t, manager.RunUpdateCycle(t.Context()))
	newRevision := manager.Revision()
	require.NotEqual(t, oldRevision, newRevision)
	checkServing(newRevision, false)
	require.NoError(t, manager.RunUpdateCycle(t.Context()))
	checkServing(newRevision, false)
	require.Len(t, watch.Events, 1)
	event := <-watch.Events
	require.Equal(t, opa.EventTypePolicyUpdated, event.Type)
	require.Equal(t, oldRevision, event.PreviousRevision)
	require.Equal(t, newRevision, event.Revision)
}

func TestFilesystemRelease_InvalidRootKeepsSnapshot(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		root    string
		wantErr error
	}{
		{name: "parent", root: "releases/../elsewhere", wantErr: coreerrs.ErrInvalidArgument},
		{name: "absolute", root: "/elsewhere", wantErr: coreerrs.ErrInvalidArgument},
		{name: "empty segment", root: "releases//1", wantErr: coreerrs.ErrInvalidArgument},
		{name: "empty root", wantErr: coreerrs.ErrInvalidArgument},
		{name: "symlink escape", root: "releases/1"},
		{name: "missing", root: "releases/missing", wantErr: os.ErrNotExist},
		{name: "incomplete", root: "releases/incomplete", wantErr: ErrMissingPolicyFile},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			parent := t.TempDir()
			base := filepath.Join(parent, "base")
			require.NoError(t, os.Mkdir(base, 0o700))
			policy := []byte("package policy\nallow := true")
			entries := map[string]string{"a.rego": corehash.SHA256HexBytes(policy)}
			require.NoError(t, os.WriteFile(filepath.Join(base, "a.rego"), policy, 0o600))
			publishFilesystemManifest(t, base, entries)
			source, err := New(base, WithReleaseManifest("release.json"))
			require.NoError(t, err)
			manager, err := opa.NewManager(t.Context(), source, "data.policy.allow")
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, manager.Close()) })
			revision := manager.Revision()

			outside := filepath.Join(parent, "outside")
			require.NoError(t, os.Mkdir(outside, 0o700))
			require.NoError(t, os.WriteFile(filepath.Join(outside, "a.rego"), policy, 0o600))
			require.NoError(t, os.MkdirAll(filepath.Join(base, "releases/incomplete"), 0o700))
			require.NoError(t, os.Symlink("../../outside", filepath.Join(base, "releases/1")))
			publishFilesystemManifest(t, base, map[string]any{"root": tc.root, "files": entries})
			err = manager.RunUpdateCycle(t.Context())
			if tc.wantErr == nil {
				require.ErrorAs(t, err, new(*os.PathError))
			} else {
				require.ErrorIs(t, err, tc.wantErr)
			}
			require.Equal(t, health.StatusNotServing, manager.CheckHealth(t.Context()))
			require.Equal(t, revision, manager.Revision())
			result, err := manager.Evaluator().Evaluate(t.Context(), nil)
			require.NoError(t, err)
			require.True(t, result.Allow)
		})
	}
}

func publishFilesystemManifest(t testing.TB, dir string, manifest any) {
	t.Helper()
	payload, err := json.Marshal(manifest)
	require.NoError(t, err)
	staged := filepath.Join(dir, ".release.tmp")
	require.NoError(t, os.WriteFile(staged, payload, 0o600))
	require.NoError(t, os.Rename(staged, filepath.Join(dir, "release.json")))
}
