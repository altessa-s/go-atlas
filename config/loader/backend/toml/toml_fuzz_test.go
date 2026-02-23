// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package toml_test

import (
	"bytes"
	"testing"

	"github.com/altessa-s/go-atlas/config/loader/backend/toml"
)

func FuzzBackend_Decode(f *testing.F) {
	// Seed the fuzzer with some valid and invalid TOML examples
	f.Add([]byte("key = \"value\""))
	f.Add([]byte("[section]\nnum = 42"))
	f.Add([]byte(""))
	f.Add([]byte("invalid"))
	f.Add([]byte("key = ["))
	f.Add([]byte("[[array]]\nid = 1"))
	f.Add([]byte("nested.key = \"value\""))

	f.Fuzz(func(t *testing.T, data []byte) {
		// The test should not panic, regardless of input
		b := &toml.Backend{}
		reader := bytes.NewReader(data)

		var target struct {
			Key   string `toml:"key"`
			Num   int    `toml:"num"`
			Value any    `toml:"value"`
		}

		// We don't care if it errors, just that it doesn't panic
		_ = b.Decode(reader, &target)
	})
}
