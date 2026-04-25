// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package toml_test

import (
	"strings"
	"testing"

	"github.com/altessa-s/go-atlas/config/loader/backend/toml"
)

func BenchmarkBackend_Decode(b *testing.B) {
	backend := &toml.Backend{}
	input := `key = "value"
name = "test"
count = 100

[section]
num = 42
enabled = true

[nested.deep]
value = "nested"
items = [1, 2, 3, 4, 5]

[[array]]
id = 1
name = "first"

[[array]]
id = 2
name = "second"
`

	type Config struct {
		Key     string `toml:"key"`
		Name    string `toml:"name"`
		Count   int    `toml:"count"`
		Section struct {
			Num     int  `toml:"num"`
			Enabled bool `toml:"enabled"`
		} `toml:"section"`
		Nested struct {
			Deep struct {
				Value string `toml:"value"`
				Items []int  `toml:"items"`
			} `toml:"deep"`
		} `toml:"nested"`
		Array []struct {
			ID   int    `toml:"id"`
			Name string `toml:"name"`
		} `toml:"array"`
	}

	b.ResetTimer()
	for b.Loop() {
		var cfg Config
		reader := strings.NewReader(input)
		if err := backend.Decode(reader, &cfg); err != nil {
			b.Fatalf("Decode failed: %v", err)
		}
	}
}
