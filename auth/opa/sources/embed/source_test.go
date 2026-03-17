// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package embed_test

import (
	"errors"
	"testing"
	"testing/fstest"

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
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	if source == nil {
		t.Fatal("New() returned nil source")
	}
	defer source.Close()
}

func TestNew_RootDirectory(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"test.rego": {Data: []byte("package test\n")},
	}

	source, err := embedsrc.New(fsys, ".")
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	if source == nil {
		t.Fatal("New() returned nil source")
	}
	defer source.Close()
}

func TestNew_EmptyDir(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"test.rego": {Data: []byte("package test\n")},
	}

	source, err := embedsrc.New(fsys, "")
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	if source == nil {
		t.Fatal("New() returned nil source")
	}
	defer source.Close()
}

func TestNew_NilFS(t *testing.T) {
	t.Parallel()

	_, err := embedsrc.New(nil, ".")
	if err == nil {
		t.Fatal("New() with nil fs.FS should fail")
	}
}

func TestNew_NonexistentDir(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"other/test.rego": {Data: []byte("package test\n")},
	}

	_, err := embedsrc.New(fsys, "nonexistent")
	if err == nil {
		t.Fatal("New() with nonexistent dir should fail")
	}
}

func TestSource_Name(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"policies/test.rego": {Data: []byte("package test\n")},
	}

	source, err := embedsrc.New(fsys, "policies")
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer source.Close()

	name := source.Name()
	expected := "embed:policies"
	if name != expected {
		t.Errorf("Name() = %q, want %q", name, expected)
	}
}

func TestSource_Dir(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"policies/test.rego": {Data: []byte("package test\n")},
	}

	source, err := embedsrc.New(fsys, "policies")
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer source.Close()

	dir := source.Dir()
	if dir != "policies" {
		t.Errorf("Dir() = %q, want %q", dir, "policies")
	}
}

func TestSource_Extensions(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"policies/test.rego": {Data: []byte("package test\n")},
	}

	source, err := embedsrc.New(fsys, "policies")
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer source.Close()

	exts := source.Extensions()
	if len(exts) != 1 {
		t.Fatalf("Extensions() returned %d extensions, want 1", len(exts))
	}
	if exts[0] != ".rego" {
		t.Errorf("Extensions()[0] = %q, want %q", exts[0], ".rego")
	}
}

func TestSource_Extensions_Custom(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"policies/test.rego": {Data: []byte("package test\n")},
	}

	source, err := embedsrc.New(fsys, "policies", embedsrc.WithExtensions(".rego", ".json"))
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer source.Close()

	exts := source.Extensions()
	if len(exts) != 2 {
		t.Fatalf("Extensions() returned %d extensions, want 2", len(exts))
	}

	extMap := make(map[string]bool)
	for _, ext := range exts {
		extMap[ext] = true
	}
	if !extMap[".rego"] {
		t.Error("Extensions() missing .rego")
	}
	if !extMap[".json"] {
		t.Error("Extensions() missing .json")
	}
}

func TestSource_Fetch(t *testing.T) {
	t.Parallel()

	policyContent := []byte("package test\ndefault allow = false")
	fsys := fstest.MapFS{
		"policies/test.rego": {Data: policyContent},
	}

	source, err := embedsrc.New(fsys, "policies")
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer source.Close()

	bundle, err := source.Fetch(t.Context())
	if err != nil {
		t.Fatalf("Fetch() failed: %v", err)
	}

	if bundle == nil {
		t.Fatal("Fetch() returned nil bundle")
	}

	if len(bundle.Modules) != 1 {
		t.Errorf("bundle has %d modules, want 1", len(bundle.Modules))
	}

	if bundle.Revision == "" {
		t.Error("bundle.Revision is empty")
	}
}

func TestSource_Fetch_MultipleFiles(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"policies/auth.rego":  {Data: []byte("package auth\ndefault allow = false")},
		"policies/utils.rego": {Data: []byte("package utils\nhelper = true")},
	}

	source, err := embedsrc.New(fsys, "policies")
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer source.Close()

	bundle, err := source.Fetch(t.Context())
	if err != nil {
		t.Fatalf("Fetch() failed: %v", err)
	}

	if len(bundle.Modules) != 2 {
		t.Errorf("bundle has %d modules, want 2", len(bundle.Modules))
	}
}

func TestSource_Fetch_NestedDirectories(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"policies/auth/main.rego":    {Data: []byte("package auth\ndefault allow = false")},
		"policies/auth/helpers.rego": {Data: []byte("package auth\nhelper = true")},
		"policies/rbac/roles.rego":   {Data: []byte("package rbac\nrole = \"admin\"")},
	}

	source, err := embedsrc.New(fsys, "policies")
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer source.Close()

	bundle, err := source.Fetch(t.Context())
	if err != nil {
		t.Fatalf("Fetch() failed: %v", err)
	}

	if len(bundle.Modules) != 3 {
		t.Errorf("bundle has %d modules, want 3", len(bundle.Modules))
	}
}

func TestSource_Fetch_Empty(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"policies/readme.txt": {Data: []byte("no policies here")},
	}

	source, err := embedsrc.New(fsys, "policies")
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer source.Close()

	_, err = source.Fetch(t.Context())
	if err == nil {
		t.Fatal("Fetch() with no policy files should fail")
	}

	if !errors.Is(err, opa.ErrNoPolicyFiles) {
		t.Errorf("Fetch() error = %v, want ErrNoPolicyFiles", err)
	}
}

func TestSource_Fetch_Closed(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"policies/test.rego": {Data: []byte("package test\n")},
	}

	source, err := embedsrc.New(fsys, "policies")
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	if err := source.Close(); err != nil {
		t.Fatalf("Close() failed: %v", err)
	}

	_, err = source.Fetch(t.Context())
	if err == nil {
		t.Fatal("Fetch() after Close() should fail")
	}

	if !errors.Is(err, opa.ErrSourceClosed) {
		t.Errorf("Fetch() error = %v, want ErrSourceClosed", err)
	}
}

func TestSource_Fetch_WithData(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"policies/test.rego": {Data: []byte("package test\ndefault allow = false")},
		"policies/data.json": {Data: []byte(`{"users": ["alice", "bob"]}`)},
	}

	source, err := embedsrc.New(fsys, "policies", embedsrc.WithIncludeData())
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer source.Close()

	bundle, err := source.Fetch(t.Context())
	if err != nil {
		t.Fatalf("Fetch() failed: %v", err)
	}

	if bundle == nil {
		t.Fatal("Fetch() returned nil bundle")
	}

	if len(bundle.Modules) != 1 {
		t.Errorf("bundle has %d modules, want 1", len(bundle.Modules))
	}

	if bundle.Data == nil {
		t.Fatal("bundle.Data is nil, expected data to be loaded")
	}

	if len(bundle.Data) == 0 {
		t.Error("bundle.Data is empty, expected at least one entry")
	}
}

func TestSource_Close_Idempotent(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"policies/test.rego": {Data: []byte("package test\n")},
	}

	source, err := embedsrc.New(fsys, "policies")
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	if err := source.Close(); err != nil {
		t.Fatalf("first Close() failed: %v", err)
	}

	if err := source.Close(); err != nil {
		t.Fatalf("second Close() failed: %v", err)
	}
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
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer source.Close()

	bundle, err := source.Fetch(t.Context())
	if err != nil {
		t.Fatalf("Fetch() failed: %v", err)
	}

	if len(bundle.Modules) != 2 {
		t.Errorf("bundle has %d modules, want 2", len(bundle.Modules))
	}
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
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer source.Close()

	_, err = source.Fetch(t.Context())
	if err == nil {
		t.Fatal("Fetch() should fail with checksum mismatch")
	}

	if !errors.Is(err, embedsrc.ErrChecksumMismatch) {
		t.Errorf("Fetch() error = %v, want ErrChecksumMismatch", err)
	}
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
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer source.Close()

	_, err = source.Fetch(t.Context())
	if err == nil {
		t.Fatal("Fetch() should fail with missing file")
	}

	if !errors.Is(err, embedsrc.ErrMissingPolicyFile) {
		t.Errorf("Fetch() error = %v, want ErrMissingPolicyFile", err)
	}
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
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer source.Close()

	_, err = source.Fetch(t.Context())
	if err == nil {
		t.Fatal("Fetch() should fail with unexpected file")
	}

	if !errors.Is(err, embedsrc.ErrUnexpectedPolicyFile) {
		t.Errorf("Fetch() error = %v, want ErrUnexpectedPolicyFile", err)
	}
}
