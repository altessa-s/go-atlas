// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package yaml3_test

import (
	"strings"
	"testing"

	"github.com/altessa-s/go-atlas/config/loader/backend/yaml3"
)

func BenchmarkBackend_Decode(b *testing.B) {
	backend := &yaml3.Backend{}
	yamlContent := `
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

	b.ResetTimer()
	for b.Loop() {
		var result map[string]any
		reader := strings.NewReader(yamlContent)
		if err := backend.Decode(reader, &result); err != nil {
			b.Fatalf("Decode error: %v", err)
		}
	}
}
