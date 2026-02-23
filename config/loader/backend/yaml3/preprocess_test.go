// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package yaml3_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altessa-s/go-atlas/config/loader/backend/yaml3"
)

func TestBackend_Preprocess_SimpleInclude(t *testing.T) {
	tmpDir := t.TempDir()

	subYaml := filepath.Join(tmpDir, "sub.yaml")
	subContent := "included: value\ndata: 123"
	if err := os.WriteFile(subYaml, []byte(subContent), 0644); err != nil {
		t.Fatalf("failed to create sub.yaml: %v", err)
	}

	mainContent := "main: config\n!include sub.yaml\nafter: include"

	backend := &yaml3.Backend{}
	result, err := backend.Preprocess(mainContent, tmpDir, tmpDir)
	if err != nil {
		t.Fatalf("Preprocess() error = %v", err)
	}

	if !strings.Contains(result, "included: value") {
		t.Errorf("result does not contain included content")
	}
	if !strings.Contains(result, "data: 123") {
		t.Errorf("result does not contain all included content")
	}
	if !strings.Contains(result, "main: config") {
		t.Errorf("result does not contain original content")
	}
}

func TestBackend_Preprocess_NestedInclude(t *testing.T) {
	tmpDir := t.TempDir()

	deepYaml := filepath.Join(tmpDir, "deep.yaml")
	deepContent := "deepest: value"
	if err := os.WriteFile(deepYaml, []byte(deepContent), 0644); err != nil {
		t.Fatalf("failed to create deep.yaml: %v", err)
	}

	subYaml := filepath.Join(tmpDir, "sub.yaml")
	subContent := "sub: value\n!include deep.yaml"
	if err := os.WriteFile(subYaml, []byte(subContent), 0644); err != nil {
		t.Fatalf("failed to create sub.yaml: %v", err)
	}

	mainContent := "main: config\n!include sub.yaml"

	backend := &yaml3.Backend{}
	result, err := backend.Preprocess(mainContent, tmpDir, tmpDir)
	if err != nil {
		t.Fatalf("Preprocess() error = %v", err)
	}

	if !strings.Contains(result, "deepest: value") {
		t.Errorf("result does not contain nested included content")
	}
	if !strings.Contains(result, "sub: value") {
		t.Errorf("result does not contain sub content")
	}
}

func TestBackend_Preprocess_IndentedInclude(t *testing.T) {
	tmpDir := t.TempDir()

	subYaml := filepath.Join(tmpDir, "sub.yaml")
	subContent := "key1: value1\nkey2: value2"
	if err := os.WriteFile(subYaml, []byte(subContent), 0644); err != nil {
		t.Fatalf("failed to create sub.yaml: %v", err)
	}

	mainContent := "root:\n  !include sub.yaml"

	backend := &yaml3.Backend{}
	result, err := backend.Preprocess(mainContent, tmpDir, tmpDir)
	if err != nil {
		t.Fatalf("Preprocess() error = %v", err)
	}

	if !strings.Contains(result, "  key1: value1") {
		t.Errorf("result does not contain properly indented content: %s", result)
	}
	if !strings.Contains(result, "  key2: value2") {
		t.Errorf("result does not contain all indented content: %s", result)
	}
}

func TestBackend_Preprocess_NoIncludes(t *testing.T) {
	tmpDir := t.TempDir()
	content := "key: value\nsection:\n  nested: data"

	backend := &yaml3.Backend{}
	result, err := backend.Preprocess(content, tmpDir, tmpDir)
	if err != nil {
		t.Fatalf("Preprocess() error = %v", err)
	}

	// Result should have content plus trailing newlines from scanner
	if !strings.Contains(result, "key: value") {
		t.Errorf("result missing original content")
	}
	if !strings.Contains(result, "nested: data") {
		t.Errorf("result missing original nested content")
	}
}

func TestBackend_Preprocess_InvalidExtension(t *testing.T) {
	tmpDir := t.TempDir()

	txtFile := filepath.Join(tmpDir, "file.txt")
	if err := os.WriteFile(txtFile, []byte("text content"), 0644); err != nil {
		t.Fatalf("failed to create file.txt: %v", err)
	}

	mainContent := "!include file.txt"

	backend := &yaml3.Backend{}
	_, err := backend.Preprocess(mainContent, tmpDir, tmpDir)
	if err == nil {
		t.Fatal("expected error for invalid extension, got nil")
	}

	if !strings.Contains(err.Error(), "invalid extension") {
		t.Errorf("error message = %v, want 'invalid extension'", err)
	}
}

func TestBackend_Preprocess_PathTraversal(t *testing.T) {
	tmpDir := t.TempDir()

	mainContent := "!include ../../../etc/passwd.yaml"

	backend := &yaml3.Backend{}
	_, err := backend.Preprocess(mainContent, tmpDir, tmpDir)
	if err == nil {
		t.Fatal("expected error for path traversal, got nil")
	}

	if !strings.Contains(err.Error(), "security error") {
		t.Errorf("error message = %v, want 'security error'", err)
	}
}

func TestBackend_Preprocess_CircularInclude(t *testing.T) {
	tmpDir := t.TempDir()

	aYaml := filepath.Join(tmpDir, "a.yaml")
	bYaml := filepath.Join(tmpDir, "b.yaml")

	aContent := "a: value\n!include b.yaml"
	bContent := "b: value\n!include a.yaml"

	if err := os.WriteFile(aYaml, []byte(aContent), 0644); err != nil {
		t.Fatalf("failed to create a.yaml: %v", err)
	}
	if err := os.WriteFile(bYaml, []byte(bContent), 0644); err != nil {
		t.Fatalf("failed to create b.yaml: %v", err)
	}

	mainContent := "!include a.yaml"

	backend := &yaml3.Backend{}
	_, err := backend.Preprocess(mainContent, tmpDir, tmpDir)
	if err == nil {
		t.Fatal("expected error for circular include, got nil")
	}

	if !strings.Contains(err.Error(), "circular include") {
		t.Errorf("error message = %v, want 'circular include'", err)
	}
}

func TestBackend_Preprocess_FileNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	mainContent := "!include nonexistent.yaml"

	backend := &yaml3.Backend{}
	_, err := backend.Preprocess(mainContent, tmpDir, tmpDir)
	if err == nil {
		t.Fatal("expected error for file not found, got nil")
	}

	if !strings.Contains(err.Error(), "failed to read included file") {
		t.Errorf("error message = %v, want 'failed to read included file'", err)
	}
}

func TestBackend_Preprocess_MaxDepth(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a chain of 11 includes to exceed maxIncludeDepth of 10
	for i := range 11 {
		filename := filepath.Join(tmpDir, yamlName(i))
		var content string
		if i == 10 {
			content = "final: value"
		} else {
			content = "!include " + yamlName(i+1)
		}
		if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
			t.Fatalf("failed to create %s: %v", filename, err)
		}
	}

	mainContent := "!include " + yamlName(0)

	backend := &yaml3.Backend{}
	_, err := backend.Preprocess(mainContent, tmpDir, tmpDir)
	if err == nil {
		t.Fatal("expected error for max depth exceeded, got nil")
	}

	if !strings.Contains(err.Error(), "max include depth") {
		t.Errorf("error message = %v, want 'max include depth'", err)
	}
}

func yamlName(i int) string {
	return "level" + string(rune('0'+i)) + ".yaml"
}
