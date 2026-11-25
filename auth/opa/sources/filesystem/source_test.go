// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filesystem_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/opa"
	"github.com/altessa-s/go-atlas/auth/opa/sources/filesystem"

	corehash "github.com/altessa-s/go-atlas-core/encoding/hash"
	embedsrc "github.com/altessa-s/go-atlas/auth/opa/sources/embed"
)

func TestNew_ValidDirectory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	policyFile := filepath.Join(dir, "test.rego")
	require.NoError(t, os.WriteFile(policyFile, []byte("package test\n"), 0o600))

	source, err := filesystem.New(dir, writeManifest(t, dir, "test.rego"))
	require.NoError(t, err)
	require.NotNil(t, source)
	defer source.Close()
}

func TestNew_ValidFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	policyFile := filepath.Join(dir, "test.rego")
	require.NoError(t, os.WriteFile(policyFile, []byte("package test\n"), 0o600))

	source, err := filesystem.New(policyFile, writeManifest(t, dir, "test.rego"))
	require.NoError(t, err)
	require.NotNil(t, source)
	defer source.Close()
}

func TestNew_InvalidPath(t *testing.T) {
	t.Parallel()

	_, err := filesystem.New("/nonexistent/path/to/nowhere", filesystem.WithReleaseManifest("release.json"))
	require.Error(t, err, "New() with nonexistent path should fail")
}

func TestNew_InvalidFileExtension(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	txtFile := filepath.Join(dir, "test.txt")
	require.NoError(t, os.WriteFile(txtFile, []byte("test"), 0o600))

	_, err := filesystem.New(txtFile, filesystem.WithReleaseManifest("release.json"))
	require.Error(t, err, "New() with .txt file should fail")
}

func TestSource_Name(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	policyFile := filepath.Join(dir, "test.rego")
	require.NoError(t, os.WriteFile(policyFile, []byte("package test\n"), 0o600))

	source, err := filesystem.New(dir, writeManifest(t, dir, "test.rego"))
	require.NoError(t, err)
	defer source.Close()

	require.Equal(t, "filesystem:"+dir, source.Name())
}

func TestSource_Path(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	policyFile := filepath.Join(dir, "test.rego")
	require.NoError(t, os.WriteFile(policyFile, []byte("package test\n"), 0o600))

	source, err := filesystem.New(dir, writeManifest(t, dir, "test.rego"))
	require.NoError(t, err)
	defer source.Close()

	require.Equal(t, dir, source.Path())
}

func TestSource_Extensions(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	policyFile := filepath.Join(dir, "test.rego")
	require.NoError(t, os.WriteFile(policyFile, []byte("package test\n"), 0o600))

	source, err := filesystem.New(dir, writeManifest(t, dir, "test.rego"))
	require.NoError(t, err)
	defer source.Close()

	exts := source.Extensions()
	require.Len(t, exts, 1)
	require.Equal(t, ".rego", exts[0])
}

func TestSource_Extensions_Custom(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	policyFile := filepath.Join(dir, "test.rego")
	require.NoError(t, os.WriteFile(policyFile, []byte("package test\n"), 0o600))

	source, err := filesystem.New(dir, writeManifest(t, dir, "test.rego"), filesystem.WithExtensions(".rego", ".json"))
	require.NoError(t, err)
	defer source.Close()

	exts := source.Extensions()
	require.Len(t, exts, 2)

	// Extensions order may vary, check both are present
	extMap := make(map[string]bool)
	for _, ext := range exts {
		extMap[ext] = true
	}
	require.True(t, extMap[".rego"], "Extensions() missing .rego")
	require.True(t, extMap[".json"], "Extensions() missing .json")
}

func TestSource_Fetch(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	policyContent := []byte("package test\ndefault allow = false")
	policyFile := filepath.Join(dir, "test.rego")
	require.NoError(t, os.WriteFile(policyFile, policyContent, 0o600))

	source, err := filesystem.New(dir, writeManifest(t, dir, "test.rego"))
	require.NoError(t, err)
	defer source.Close()

	ctx := t.Context()
	bundle, err := source.Fetch(ctx)
	require.NoError(t, err)
	require.NotNil(t, bundle)
	require.Len(t, bundle.Modules, 1)
	require.NotEmpty(t, bundle.Revision)
}

func TestSource_Fetch_Empty(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// Create directory but no .rego files

	source, err := filesystem.New(dir, writeManifest(t, dir))
	require.NoError(t, err)
	defer source.Close()

	ctx := t.Context()
	_, err = source.Fetch(ctx)
	require.Error(t, err)
	require.True(t, errors.Is(err, opa.ErrNoPolicyFiles), "Fetch() error = %v, want ErrNoPolicyFiles", err)
}

func TestSource_Fetch_Closed(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	policyFile := filepath.Join(dir, "test.rego")
	require.NoError(t, os.WriteFile(policyFile, []byte("package test\n"), 0o600))

	source, err := filesystem.New(dir, writeManifest(t, dir, "test.rego"))
	require.NoError(t, err)

	require.NoError(t, source.Close())

	ctx := t.Context()
	_, err = source.Fetch(ctx)
	require.Error(t, err)
	require.True(t, errors.Is(err, opa.ErrSourceClosed), "Fetch() error = %v, want ErrSourceClosed", err)
}

func TestSource_Fetch_WithData(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	policyContent := []byte("package test\ndefault allow = false")
	policyFile := filepath.Join(dir, "test.rego")
	require.NoError(t, os.WriteFile(policyFile, policyContent, 0o600))

	jsonContent := []byte(`9007199254740993`)
	jsonFile := filepath.Join(dir, "data.json")
	require.NoError(t, os.WriteFile(jsonFile, jsonContent, 0o600))

	source, err := filesystem.New(dir, writeManifest(t, dir, "test.rego", "data.json"), filesystem.WithIncludeData())
	require.NoError(t, err)

	ctx := t.Context()
	bundle, err := source.Fetch(ctx)
	require.NoError(t, err)
	require.NotNil(t, bundle)
	require.Len(t, bundle.Modules, 1)
	require.NotNil(t, bundle.Data, "bundle.Data is nil, expected data to be loaded")
	require.NotEmpty(t, bundle.Data, "bundle.Data is empty, expected at least one entry")
	require.Equal(t, json.Number("9007199254740993"), bundle.Data["data"])
	m, err := opa.NewManager(ctx, source, "data.data == 9007199254740993")
	require.NoError(t, err)
	t.Cleanup(func() { _ = m.Close() })
	result, err := m.Evaluator().Evaluate(ctx, nil)
	require.NoError(t, err)
	require.True(t, result.Allow)
	require.NoError(t, os.WriteFile(jsonFile, []byte("1 2"), 0o600))
	writeManifest(t, dir, "test.rego", "data.json")
	_, err = source.Fetch(ctx)
	require.Error(t, err)
}

func TestSource_Close_Idempotent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	policyFile := filepath.Join(dir, "test.rego")
	require.NoError(t, os.WriteFile(policyFile, []byte("package test\n"), 0o600))

	source, err := filesystem.New(dir, writeManifest(t, dir, "test.rego"))
	require.NoError(t, err)

	require.NoError(t, source.Close(), "first Close() failed")
	require.NoError(t, source.Close(), "second Close() failed")
}

func TestSource_Fetch_ChecksumValid(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	policy1 := []byte("package test\ndefault allow = false\n")
	policy2 := []byte("package utils\nhelper = true\n")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "test.rego"), policy1, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "utils.rego"), policy2, 0o600))

	checksums := map[string]string{
		"test.rego":  corehash.SHA256HexBytes(policy1),
		"utils.rego": corehash.SHA256HexBytes(policy2),
	}

	source, err := filesystem.New(dir, filesystem.WithChecksums(checksums))
	require.NoError(t, err)
	defer source.Close()

	bundle, err := source.Fetch(t.Context())
	require.NoError(t, err)
	require.Len(t, bundle.Modules, 2)
}

func TestSource_Fetch_ChecksumMismatch(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	policy := []byte("package test\ndefault allow = false\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "test.rego"), policy, 0o600))

	checksums := map[string]string{
		"test.rego": "0000000000000000000000000000000000000000000000000000000000000000",
	}

	source, err := filesystem.New(dir, filesystem.WithChecksums(checksums))
	require.NoError(t, err)
	defer source.Close()

	_, err = source.Fetch(t.Context())
	require.Error(t, err)
	require.True(t, errors.Is(err, filesystem.ErrChecksumMismatch), "Fetch() error = %v, want ErrChecksumMismatch", err)
}

func TestSource_Fetch_ChecksumMissingFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	policy := []byte("package test\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "test.rego"), policy, 0o600))

	checksums := map[string]string{
		"test.rego":    corehash.SHA256HexBytes(policy),
		"missing.rego": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}

	source, err := filesystem.New(dir, filesystem.WithChecksums(checksums))
	require.NoError(t, err)
	defer source.Close()

	_, err = source.Fetch(t.Context())
	require.Error(t, err)
	require.True(t, errors.Is(err, filesystem.ErrMissingPolicyFile), "Fetch() error = %v, want ErrMissingPolicyFile", err)
}

func TestSource_Fetch_ChecksumUnexpectedFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	policy1 := []byte("package test\n")
	policy2 := []byte("package extra\n")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "test.rego"), policy1, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "extra.rego"), policy2, 0o600))

	// Only include test.rego in checksums — extra.rego is unexpected.
	checksums := map[string]string{
		"test.rego": corehash.SHA256HexBytes(policy1),
	}

	source, err := filesystem.New(dir, filesystem.WithChecksums(checksums))
	require.NoError(t, err)
	defer source.Close()

	_, err = source.Fetch(t.Context())
	require.Error(t, err)
	require.True(t, errors.Is(err, filesystem.ErrUnexpectedPolicyFile), "Fetch() error = %v, want ErrUnexpectedPolicyFile", err)
}

func TestJSONManifestReload(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"filesystem", "embed"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			require.NoError(t, os.Mkdir(filepath.Join(dir, "nested"), 0o700))
			const policy = "package t\nallow := data.nested.grant"
			require.NoError(t, os.WriteFile(filepath.Join(dir, "main.rego"), []byte(policy), 0o600))
			file := filepath.Join(dir, "nested", "grant.json")
			require.NoError(t, os.WriteFile(file, []byte("true"), 0o600))
			manifest := map[string]string{"main.rego": corehash.SHA256HexBytes([]byte(policy)), "nested/grant.json": corehash.SHA256HexBytes([]byte("true"))}
			var source opa.PolicySource
			var err error
			mismatch, unexpected, missing := filesystem.ErrChecksumMismatch, filesystem.ErrUnexpectedPolicyFile, filesystem.ErrMissingPolicyFile
			if name == "embed" {
				source, err = embedsrc.New(os.DirFS(dir), ".", embedsrc.WithIncludeData(), embedsrc.WithChecksums(manifest))
				mismatch, unexpected, missing = embedsrc.ErrChecksumMismatch, embedsrc.ErrUnexpectedPolicyFile, embedsrc.ErrMissingPolicyFile
			} else {
				source, err = filesystem.New(dir, filesystem.WithIncludeData(), filesystem.WithChecksums(manifest))
			}
			require.NoError(t, err)
			m, err := opa.NewManager(t.Context(), source, "data.t.allow")
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(file, []byte("{"), 0o600))
			require.ErrorIs(t, m.RunUpdateCycle(t.Context()), mismatch)
			require.NoError(t, os.Rename(file, filepath.Join(dir, "unexpected.json")))
			require.ErrorIs(t, m.RunUpdateCycle(t.Context()), unexpected)
			require.NoError(t, os.Remove(filepath.Join(dir, "unexpected.json")))
			require.ErrorIs(t, m.RunUpdateCycle(t.Context()), missing)
			result, err := m.Evaluator().Evaluate(t.Context(), nil)
			require.NoError(t, err)
			require.True(t, result.Allow)
			require.NoError(t, m.Close())
		})
	}
}

func writeManifest(t testing.TB, dir string, names ...string) filesystem.Option {
	t.Helper()

	checksums := make(map[string]string, len(names))
	for _, name := range names {
		content, err := os.ReadFile(filepath.Join(dir, name))
		require.NoError(t, err)
		checksums[name] = corehash.SHA256HexBytes(content)
	}
	payload, err := json.Marshal(checksums)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "release.json"), payload, 0o600))
	return filesystem.WithReleaseManifest("release.json")
}

func TestNew_RequiresReleaseManifest(t *testing.T) {
	t.Parallel()

	_, err := filesystem.New(t.TempDir())
	require.ErrorIs(t, err, opa.ErrReleaseManifestRequired)
}
