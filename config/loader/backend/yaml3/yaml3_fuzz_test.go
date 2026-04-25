// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package yaml3_test

import (
	"bytes"
	"testing"

	"github.com/altessa-s/go-atlas/config/loader/backend/yaml3"
)

func FuzzBackend_Decode(f *testing.F) {
	// Seed corpus with some valid and invalid YAML examples
	f.Add([]byte("key: value"))
	f.Add([]byte("key: value\nsection:\n  num: 42"))
	f.Add([]byte(""))
	f.Add([]byte("invalid: [unclosed"))
	f.Add([]byte("---\nkey: value"))

	backend := &yaml3.Backend{}

	f.Fuzz(func(t *testing.T, data []byte) {
		var result map[string]any
		reader := bytes.NewReader(data)
		// We don't care about errors, just that it doesn't panic
		_ = backend.Decode(reader, &result)
	})
}

func FuzzBackend_Preprocess(f *testing.F) {
	// Seed corpus with various preprocess patterns
	f.Add("key: value")
	f.Add("!include file.yaml")
	f.Add("  !include file.yaml")
	f.Add("key: value\n!include sub.yaml\nafter: data")
	f.Add("")

	f.Fuzz(func(t *testing.T, content string) {
		tmpDir := t.TempDir()
		backend := &yaml3.Backend{}
		// We don't care about errors, just that it doesn't panic
		_, _ = backend.Preprocess(content, tmpDir, tmpDir)
	})
}
