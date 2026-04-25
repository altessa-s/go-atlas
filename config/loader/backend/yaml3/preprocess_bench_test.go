// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package yaml3_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/altessa-s/go-atlas/config/loader/backend/yaml3"
)

func BenchmarkBackend_Preprocess_NoIncludes(b *testing.B) {
	backend := &yaml3.Backend{}
	content := `
key: value
section:
  num: 42
  str: hello
  bool: true
  nested:
    deep: data
    list:
      - item1
      - item2
      - item3
another:
  field1: value1
  field2: value2
  field3: value3
`
	tmpDir := b.TempDir()

	b.ResetTimer()
	for b.Loop() {
		_, err := backend.Preprocess(content, tmpDir, tmpDir)
		if err != nil {
			b.Fatalf("Preprocess error: %v", err)
		}
	}
}

func BenchmarkBackend_Preprocess_WithInclude(b *testing.B) {
	tmpDir := b.TempDir()

	subYaml := filepath.Join(tmpDir, "sub.yaml")
	subContent := `
included: value
data: 123
nested:
  key: value
`
	if err := os.WriteFile(subYaml, []byte(subContent), 0644); err != nil {
		b.Fatalf("failed to create sub.yaml: %v", err)
	}

	mainContent := `
main: config
section:
  !include sub.yaml
after: include
`

	backend := &yaml3.Backend{}

	b.ResetTimer()
	for b.Loop() {
		_, err := backend.Preprocess(mainContent, tmpDir, tmpDir)
		if err != nil {
			b.Fatalf("Preprocess error: %v", err)
		}
	}
}
