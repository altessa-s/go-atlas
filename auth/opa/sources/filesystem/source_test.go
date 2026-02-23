// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filesystem_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/auth/opa"
	"github.com/altessa-s/go-atlas/auth/opa/sources/filesystem"
)

func TestNew_ValidDirectory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	policyFile := filepath.Join(dir, "test.rego")
	if err := os.WriteFile(policyFile, []byte("package test\n"), 0o600); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	source, err := filesystem.New(dir)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	if source == nil {
		t.Fatal("New() returned nil source")
	}
	defer source.Close()
}

func TestNew_ValidFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	policyFile := filepath.Join(dir, "test.rego")
	if err := os.WriteFile(policyFile, []byte("package test\n"), 0o600); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	source, err := filesystem.New(policyFile)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	if source == nil {
		t.Fatal("New() returned nil source")
	}
	defer source.Close()
}

func TestNew_InvalidPath(t *testing.T) {
	t.Parallel()

	_, err := filesystem.New("/nonexistent/path/to/nowhere")
	if err == nil {
		t.Fatal("New() with nonexistent path should fail")
	}
}

func TestNew_InvalidFileExtension(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	txtFile := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(txtFile, []byte("test"), 0o600); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	_, err := filesystem.New(txtFile)
	if err == nil {
		t.Fatal("New() with .txt file should fail")
	}
}

func TestSource_Name(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	policyFile := filepath.Join(dir, "test.rego")
	if err := os.WriteFile(policyFile, []byte("package test\n"), 0o600); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	source, err := filesystem.New(dir)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer source.Close()

	name := source.Name()
	expected := "filesystem:" + dir
	if name != expected {
		t.Errorf("Name() = %q, want %q", name, expected)
	}
}

func TestSource_Path(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	policyFile := filepath.Join(dir, "test.rego")
	if err := os.WriteFile(policyFile, []byte("package test\n"), 0o600); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	source, err := filesystem.New(dir)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer source.Close()

	path := source.Path()
	if path != dir {
		t.Errorf("Path() = %q, want %q", path, dir)
	}
}

func TestSource_Extensions(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	policyFile := filepath.Join(dir, "test.rego")
	if err := os.WriteFile(policyFile, []byte("package test\n"), 0o600); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	source, err := filesystem.New(dir)
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

	dir := t.TempDir()
	policyFile := filepath.Join(dir, "test.rego")
	if err := os.WriteFile(policyFile, []byte("package test\n"), 0o600); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	source, err := filesystem.New(dir, filesystem.WithExtensions(".rego", ".json"))
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer source.Close()

	exts := source.Extensions()
	if len(exts) != 2 {
		t.Fatalf("Extensions() returned %d extensions, want 2", len(exts))
	}

	// Extensions order may vary, check both are present
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

	dir := t.TempDir()
	policyContent := []byte("package test\ndefault allow = false")
	policyFile := filepath.Join(dir, "test.rego")
	if err := os.WriteFile(policyFile, policyContent, 0o600); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	source, err := filesystem.New(dir)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer source.Close()

	ctx := t.Context()
	bundle, err := source.Fetch(ctx)
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

func TestSource_Fetch_Empty(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// Create directory but no .rego files

	source, err := filesystem.New(dir)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer source.Close()

	ctx := t.Context()
	_, err = source.Fetch(ctx)
	if err == nil {
		t.Fatal("Fetch() with no policy files should fail")
	}

	if !errors.Is(err, opa.ErrNoPolicyFiles) {
		t.Errorf("Fetch() error = %v, want ErrNoPolicyFiles", err)
	}
}

func TestSource_Fetch_Closed(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	policyFile := filepath.Join(dir, "test.rego")
	if err := os.WriteFile(policyFile, []byte("package test\n"), 0o600); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	source, err := filesystem.New(dir)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	if err := source.Close(); err != nil {
		t.Fatalf("Close() failed: %v", err)
	}

	ctx := t.Context()
	_, err = source.Fetch(ctx)
	if err == nil {
		t.Fatal("Fetch() after Close() should fail")
	}

	if !errors.Is(err, opa.ErrSourceClosed) {
		t.Errorf("Fetch() error = %v, want ErrSourceClosed", err)
	}
}

func TestSource_Fetch_WithData(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	policyContent := []byte("package test\ndefault allow = false")
	policyFile := filepath.Join(dir, "test.rego")
	if err := os.WriteFile(policyFile, policyContent, 0o600); err != nil {
		t.Fatalf("failed to create policy file: %v", err)
	}

	jsonContent := []byte(`{"users": ["alice", "bob"]}`)
	jsonFile := filepath.Join(dir, "data.json")
	if err := os.WriteFile(jsonFile, jsonContent, 0o600); err != nil {
		t.Fatalf("failed to create json file: %v", err)
	}

	source, err := filesystem.New(dir, filesystem.WithIncludeData())
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer source.Close()

	ctx := t.Context()
	bundle, err := source.Fetch(ctx)
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

func TestSource_Watch_Closed(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	policyFile := filepath.Join(dir, "test.rego")
	if err := os.WriteFile(policyFile, []byte("package test\n"), 0o600); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	source, err := filesystem.New(dir)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	if err := source.Close(); err != nil {
		t.Fatalf("Close() failed: %v", err)
	}

	ctx := t.Context()
	_, err = source.Watch(ctx)
	if err == nil {
		t.Fatal("Watch() after Close() should fail")
	}

	if !errors.Is(err, opa.ErrSourceClosed) {
		t.Errorf("Watch() error = %v, want ErrSourceClosed", err)
	}
}

func TestSource_Close_Idempotent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	policyFile := filepath.Join(dir, "test.rego")
	if err := os.WriteFile(policyFile, []byte("package test\n"), 0o600); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	source, err := filesystem.New(dir)
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

func TestSource_Watch(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	policyFile := filepath.Join(dir, "test.rego")
	if err := os.WriteFile(policyFile, []byte("package test\n"), 0o600); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	source, err := filesystem.New(dir)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer source.Close()

	ctx := t.Context()
	ch, err := source.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch() failed: %v", err)
	}

	if ch == nil {
		t.Fatal("Watch() returned nil channel")
	}

	// Write a new policy file to trigger a change
	newPolicyFile := filepath.Join(dir, "new.rego")
	if err := os.WriteFile(newPolicyFile, []byte("package new\n"), 0o600); err != nil {
		t.Fatalf("failed to create new policy file: %v", err)
	}

	// Wait for signal with timeout
	select {
	case <-ch:
		// Success - received signal
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for watch signal")
	}
}

func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func TestSource_Fetch_ChecksumValid(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	policy1 := []byte("package test\ndefault allow = false\n")
	policy2 := []byte("package utils\nhelper = true\n")

	if err := os.WriteFile(filepath.Join(dir, "test.rego"), policy1, 0o600); err != nil {
		t.Fatalf("write policy1: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "utils.rego"), policy2, 0o600); err != nil {
		t.Fatalf("write policy2: %v", err)
	}

	checksums := map[string]string{
		"test.rego":  sha256Hex(policy1),
		"utils.rego": sha256Hex(policy2),
	}

	source, err := filesystem.New(dir, filesystem.WithChecksums(checksums))
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

	dir := t.TempDir()

	policy := []byte("package test\ndefault allow = false\n")
	if err := os.WriteFile(filepath.Join(dir, "test.rego"), policy, 0o600); err != nil {
		t.Fatalf("write policy: %v", err)
	}

	checksums := map[string]string{
		"test.rego": "0000000000000000000000000000000000000000000000000000000000000000",
	}

	source, err := filesystem.New(dir, filesystem.WithChecksums(checksums))
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer source.Close()

	_, err = source.Fetch(t.Context())
	if err == nil {
		t.Fatal("Fetch() should fail with checksum mismatch")
	}

	if !errors.Is(err, filesystem.ErrChecksumMismatch) {
		t.Errorf("Fetch() error = %v, want ErrChecksumMismatch", err)
	}
}

func TestSource_Fetch_ChecksumMissingFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	policy := []byte("package test\n")
	if err := os.WriteFile(filepath.Join(dir, "test.rego"), policy, 0o600); err != nil {
		t.Fatalf("write policy: %v", err)
	}

	checksums := map[string]string{
		"test.rego":    sha256Hex(policy),
		"missing.rego": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}

	source, err := filesystem.New(dir, filesystem.WithChecksums(checksums))
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer source.Close()

	_, err = source.Fetch(t.Context())
	if err == nil {
		t.Fatal("Fetch() should fail with missing file")
	}

	if !errors.Is(err, filesystem.ErrMissingPolicyFile) {
		t.Errorf("Fetch() error = %v, want ErrMissingPolicyFile", err)
	}
}

func TestSource_Fetch_ChecksumUnexpectedFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	policy1 := []byte("package test\n")
	policy2 := []byte("package extra\n")

	if err := os.WriteFile(filepath.Join(dir, "test.rego"), policy1, 0o600); err != nil {
		t.Fatalf("write policy1: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "extra.rego"), policy2, 0o600); err != nil {
		t.Fatalf("write policy2: %v", err)
	}

	// Only include test.rego in checksums — extra.rego is unexpected.
	checksums := map[string]string{
		"test.rego": sha256Hex(policy1),
	}

	source, err := filesystem.New(dir, filesystem.WithChecksums(checksums))
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer source.Close()

	_, err = source.Fetch(t.Context())
	if err == nil {
		t.Fatal("Fetch() should fail with unexpected file")
	}

	if !errors.Is(err, filesystem.ErrUnexpectedPolicyFile) {
		t.Errorf("Fetch() error = %v, want ErrUnexpectedPolicyFile", err)
	}
}
