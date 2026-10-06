// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package yaml3_test

import (
	"reflect"
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

func BenchmarkBackend_DecodeKeys(b *testing.B) {
	type item struct {
		Name   string `yaml:"name"`
		Weight int    `yaml:"weight"`
	}
	type config struct {
		Name  string          `yaml:"name"`
		Items []item          `yaml:"items"`
		ByID  map[string]item `yaml:"byID"`
	}
	content := `
name: svc
items:
  - {name: a, weight: 0}
  - {name: b}
byID:
  a: {weight: 1}
  b: {name: b}
`
	t := reflect.TypeFor[config]()
	backend := &yaml3.Backend{}

	for b.Loop() {
		root, err := backend.DecodeKeys(strings.NewReader(content))
		if err != nil {
			b.Fatalf("DecodeKeys error: %v", err)
		}
		fs, _ := root.Fields(t)
		_, _ = fs[1].Elems(t.Field(1).Type)
		_, _ = fs[2].Entries(t.Field(2).Type)
	}
}

func BenchmarkBackend_DecodeStrict(b *testing.B) {
	type config struct {
		Name    string `yaml:"name"`
		Section struct {
			Num     int  `yaml:"num"`
			Enabled bool `yaml:"enabled"`
		} `yaml:"section"`
		Items []struct {
			ID int `yaml:"id"`
		} `yaml:"items"`
		Meta map[string]any `yaml:"meta"`
	}
	content := "name: svc\nsection:\n  num: 42\n  enabled: true\nitems:\n  - id: 1\nmeta:\n  free:\n    form: 1\n"
	backend := &yaml3.Backend{}

	for b.Loop() {
		var cfg config
		if err := backend.DecodeStrict(strings.NewReader(content), &cfg); err != nil {
			b.Fatalf("DecodeStrict failed: %v", err)
		}
	}
}
