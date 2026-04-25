// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package embed_test

import (
	"errors"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/opa"

	embedsrc "github.com/altessa-s/go-atlas/auth/opa/sources/embed"
	corehash "github.com/altessa-s/go-atlas/core/encoding/hash"
)

func TestNew_ValidDirectory(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"policies/test.rego": {Data: []byte("package test\n")},
	}

	source, err := embedsrc.New(fsys, "policies")
	require.NoError(t, err)
	require.NotNil(t, source)
	defer source.Close()
}

func TestNew_RootDirectory(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"test.rego": {Data: []byte("package test\n")},
	}

	source, err := embedsrc.New(fsys, ".")
	require.NoError(t, err)
	require.NotNil(t, source)
	defer source.Close()
}

func TestNew_EmptyDir(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"test.rego": {Data: []byte("package test\n")},
	}

	source, err := embedsrc.New(fsys, "")
	require.NoError(t, err)
	require.NotNil(t, source)
	defer source.Close()
}

func TestNew_NilFS(t *testing.T) {
	t.Parallel()

	_, err := embedsrc.New(nil, ".")
	require.Error(t, err, "New() with nil fs.FS should fail")
}

func TestNew_NonexistentDir(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"other/test.rego": {Data: []byte("package test\n")},
	}

	_, err := embedsrc.New(fsys, "nonexistent")
	require.Error(t, err, "New() with nonexistent dir should fail")
}

func TestSource_Name(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"policies/test.rego": {Data: []byte("package test\n")},
	}

	source, err := embedsrc.New(fsys, "policies")
	require.NoError(t, err)
	defer source.Close()

	require.Equal(t, "embed:policies", source.Name())
}

func TestSource_Dir(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"policies/test.rego": {Data: []byte("package test\n")},
	}

	source, err := embedsrc.New(fsys, "policies")
	require.NoError(t, err)
	defer source.Close()

	require.Equal(t, "policies", source.Dir())
}

func TestSource_Extensions(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"policies/test.rego": {Data: []byte("package test\n")},
	}

	source, err := embedsrc.New(fsys, "policies")
	require.NoError(t, err)
	defer source.Close()

	exts := source.Extensions()
	require.Len(t, exts, 1)
	require.Equal(t, ".rego", exts[0])
}

func TestSource_Extensions_Custom(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"policies/test.rego": {Data: []byte("package test\n")},
	}

	source, err := embedsrc.New(fsys, "policies", embedsrc.WithExtensions(".rego", ".json"))
	require.NoError(t, err)
	defer source.Close()

	exts := source.Extensions()
	require.Len(t, exts, 2)

	extMap := make(map[string]bool)
	for _, ext := range exts {
		extMap[ext] = true
	}
	require.True(t, extMap[".rego"], "Extensions() missing .rego")
	require.True(t, extMap[".json"], "Extensions() missing .json")
}

func TestSource_Fetch(t *testing.T) {
	t.Parallel()

	policyContent := []byte("package test\ndefault allow = false")
	fsys := fstest.MapFS{
		"policies/test.rego": {Data: policyContent},
	}

	source, err := embedsrc.New(fsys, "policies")
	require.NoError(t, err)
	defer source.Close()

	bundle, err := source.Fetch(t.Context())
	require.NoError(t, err)
	require.NotNil(t, bundle)
	require.Len(t, bundle.Modules, 1)
	require.NotEmpty(t, bundle.Revision)
}

func TestSource_Fetch_MultipleFiles(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"policies/auth.rego":  {Data: []byte("package auth\ndefault allow = false")},
		"policies/utils.rego": {Data: []byte("package utils\nhelper = true")},
	}

	source, err := embedsrc.New(fsys, "policies")
	require.NoError(t, err)
	defer source.Close()

	bundle, err := source.Fetch(t.Context())
	require.NoError(t, err)
	require.Len(t, bundle.Modules, 2)
}

func TestSource_Fetch_NestedDirectories(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"policies/auth/main.rego":    {Data: []byte("package auth\ndefault allow = false")},
		"policies/auth/helpers.rego": {Data: []byte("package auth\nhelper = true")},
		"policies/rbac/roles.rego":   {Data: []byte("package rbac\nrole = \"admin\"")},
	}

	source, err := embedsrc.New(fsys, "policies")
	require.NoError(t, err)
	defer source.Close()

	bundle, err := source.Fetch(t.Context())
	require.NoError(t, err)
	require.Len(t, bundle.Modules, 3)
}

func TestSource_Fetch_Empty(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"policies/readme.txt": {Data: []byte("no policies here")},
	}

	source, err := embedsrc.New(fsys, "policies")
	require.NoError(t, err)
	defer source.Close()

	_, err = source.Fetch(t.Context())
	require.Error(t, err)
	require.True(t, errors.Is(err, opa.ErrNoPolicyFiles), "Fetch() error = %v, want ErrNoPolicyFiles", err)
}

func TestSource_Fetch_Closed(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"policies/test.rego": {Data: []byte("package test\n")},
	}

	source, err := embedsrc.New(fsys, "policies")
	require.NoError(t, err)

	require.NoError(t, source.Close())

	_, err = source.Fetch(t.Context())
	require.Error(t, err)
	require.True(t, errors.Is(err, opa.ErrSourceClosed), "Fetch() error = %v, want ErrSourceClosed", err)
}

func TestSource_Fetch_WithData(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"policies/test.rego": {Data: []byte("package test\ndefault allow = false")},
		"policies/data.json": {Data: []byte(`{"users": ["alice", "bob"]}`)},
	}

	source, err := embedsrc.New(fsys, "policies", embedsrc.WithIncludeData())
	require.NoError(t, err)
	defer source.Close()

	bundle, err := source.Fetch(t.Context())
	require.NoError(t, err)
	require.NotNil(t, bundle)
	require.Len(t, bundle.Modules, 1)
	require.NotNil(t, bundle.Data, "bundle.Data is nil, expected data to be loaded")
	require.NotEmpty(t, bundle.Data, "bundle.Data is empty, expected at least one entry")
}

func TestSource_Close_Idempotent(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"policies/test.rego": {Data: []byte("package test\n")},
	}

	source, err := embedsrc.New(fsys, "policies")
	require.NoError(t, err)

	require.NoError(t, source.Close(), "first Close() failed")
	require.NoError(t, source.Close(), "second Close() failed")
}

func TestSource_Fetch_ChecksumValid(t *testing.T) {
	t.Parallel()

	policy1 := []byte("package test\ndefault allow = false\n")
	policy2 := []byte("package utils\nhelper = true\n")

	fsys := fstest.MapFS{
		"policies/test.rego":  {Data: policy1},
		"policies/utils.rego": {Data: policy2},
	}

	checksums := map[string]string{
		"test.rego":  corehash.SHA256HexBytes(policy1),
		"utils.rego": corehash.SHA256HexBytes(policy2),
	}

	source, err := embedsrc.New(fsys, "policies", embedsrc.WithChecksums(checksums))
	require.NoError(t, err)
	defer source.Close()

	bundle, err := source.Fetch(t.Context())
	require.NoError(t, err)
	require.Len(t, bundle.Modules, 2)
}

func TestSource_Fetch_ChecksumMismatch(t *testing.T) {
	t.Parallel()

	policy := []byte("package test\ndefault allow = false\n")

	fsys := fstest.MapFS{
		"policies/test.rego": {Data: policy},
	}

	checksums := map[string]string{
		"test.rego": "0000000000000000000000000000000000000000000000000000000000000000",
	}

	source, err := embedsrc.New(fsys, "policies", embedsrc.WithChecksums(checksums))
	require.NoError(t, err)
	defer source.Close()

	_, err = source.Fetch(t.Context())
	require.Error(t, err)
	require.True(t, errors.Is(err, embedsrc.ErrChecksumMismatch), "Fetch() error = %v, want ErrChecksumMismatch", err)
}

func TestSource_Fetch_ChecksumMissingFile(t *testing.T) {
	t.Parallel()

	policy := []byte("package test\n")

	fsys := fstest.MapFS{
		"policies/test.rego": {Data: policy},
	}

	checksums := map[string]string{
		"test.rego":    corehash.SHA256HexBytes(policy),
		"missing.rego": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}

	source, err := embedsrc.New(fsys, "policies", embedsrc.WithChecksums(checksums))
	require.NoError(t, err)
	defer source.Close()

	_, err = source.Fetch(t.Context())
	require.Error(t, err)
	require.True(t, errors.Is(err, embedsrc.ErrMissingPolicyFile), "Fetch() error = %v, want ErrMissingPolicyFile", err)
}

func TestSource_Fetch_ChecksumUnexpectedFile(t *testing.T) {
	t.Parallel()

	policy1 := []byte("package test\n")
	policy2 := []byte("package extra\n")

	fsys := fstest.MapFS{
		"policies/test.rego":  {Data: policy1},
		"policies/extra.rego": {Data: policy2},
	}

	// Only include test.rego in checksums — extra.rego is unexpected.
	checksums := map[string]string{
		"test.rego": corehash.SHA256HexBytes(policy1),
	}

	source, err := embedsrc.New(fsys, "policies", embedsrc.WithChecksums(checksums))
	require.NoError(t, err)
	defer source.Close()

	_, err = source.Fetch(t.Context())
	require.Error(t, err)
	require.True(t, errors.Is(err, embedsrc.ErrUnexpectedPolicyFile), "Fetch() error = %v, want ErrUnexpectedPolicyFile", err)
}
