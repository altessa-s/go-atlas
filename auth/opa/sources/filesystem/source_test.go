// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filesystem_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/opa"
	"github.com/altessa-s/go-atlas/auth/opa/sources/filesystem"

	corehash "github.com/altessa-s/go-atlas/core/encoding/hash"
)

func TestNew_ValidDirectory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	policyFile := filepath.Join(dir, "test.rego")
	require.NoError(t, os.WriteFile(policyFile, []byte("package test\n"), 0o600))

	source, err := filesystem.New(dir)
	require.NoError(t, err)
	require.NotNil(t, source)
	defer source.Close()
}

func TestNew_ValidFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	policyFile := filepath.Join(dir, "test.rego")
	require.NoError(t, os.WriteFile(policyFile, []byte("package test\n"), 0o600))

	source, err := filesystem.New(policyFile)
	require.NoError(t, err)
	require.NotNil(t, source)
	defer source.Close()
}

func TestNew_InvalidPath(t *testing.T) {
	t.Parallel()

	_, err := filesystem.New("/nonexistent/path/to/nowhere")
	require.Error(t, err, "New() with nonexistent path should fail")
}

func TestNew_InvalidFileExtension(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	txtFile := filepath.Join(dir, "test.txt")
	require.NoError(t, os.WriteFile(txtFile, []byte("test"), 0o600))

	_, err := filesystem.New(txtFile)
	require.Error(t, err, "New() with .txt file should fail")
}

func TestSource_Name(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	policyFile := filepath.Join(dir, "test.rego")
	require.NoError(t, os.WriteFile(policyFile, []byte("package test\n"), 0o600))

	source, err := filesystem.New(dir)
	require.NoError(t, err)
	defer source.Close()

	require.Equal(t, "filesystem:"+dir, source.Name())
}

func TestSource_Path(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	policyFile := filepath.Join(dir, "test.rego")
	require.NoError(t, os.WriteFile(policyFile, []byte("package test\n"), 0o600))

	source, err := filesystem.New(dir)
	require.NoError(t, err)
	defer source.Close()

	require.Equal(t, dir, source.Path())
}

func TestSource_Extensions(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	policyFile := filepath.Join(dir, "test.rego")
	require.NoError(t, os.WriteFile(policyFile, []byte("package test\n"), 0o600))

	source, err := filesystem.New(dir)
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

	source, err := filesystem.New(dir, filesystem.WithExtensions(".rego", ".json"))
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

	source, err := filesystem.New(dir)
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

	source, err := filesystem.New(dir)
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

	source, err := filesystem.New(dir)
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

	jsonContent := []byte(`{"users": ["alice", "bob"]}`)
	jsonFile := filepath.Join(dir, "data.json")
	require.NoError(t, os.WriteFile(jsonFile, jsonContent, 0o600))

	source, err := filesystem.New(dir, filesystem.WithIncludeData())
	require.NoError(t, err)
	defer source.Close()

	ctx := t.Context()
	bundle, err := source.Fetch(ctx)
	require.NoError(t, err)
	require.NotNil(t, bundle)
	require.Len(t, bundle.Modules, 1)
	require.NotNil(t, bundle.Data, "bundle.Data is nil, expected data to be loaded")
	require.NotEmpty(t, bundle.Data, "bundle.Data is empty, expected at least one entry")
}

func TestSource_Close_Idempotent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	policyFile := filepath.Join(dir, "test.rego")
	require.NoError(t, os.WriteFile(policyFile, []byte("package test\n"), 0o600))

	source, err := filesystem.New(dir)
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
