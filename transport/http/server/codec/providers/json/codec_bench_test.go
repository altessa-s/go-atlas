// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package json

import (
	"bytes"
	"testing"
)

func BenchmarkEncode_Compact(b *testing.B) {
	c := New()
	data := map[string]string{"key": "value", "name": "test"}
	var result []byte
	for b.Loop() {
		result, _ = c.Encode(data)
	}
	_ = result
}

func BenchmarkEncode_Indent(b *testing.B) {
	c := New(WithIndent(true))
	data := map[string]string{"key": "value", "name": "test"}
	var result []byte
	for b.Loop() {
		result, _ = c.Encode(data)
	}
	_ = result
}

func BenchmarkEncode_NoEscapeHTML(b *testing.B) {
	c := New(WithEscapeHTML(false))
	data := map[string]string{"html": "<b>bold</b>"}
	var result []byte
	for b.Loop() {
		result, _ = c.Encode(data)
	}
	_ = result
}

func BenchmarkDecode(b *testing.B) {
	c := New()
	input := []byte(`{"key":"value","name":"test"}`)
	for b.Loop() {
		var result map[string]string
		_ = c.Decode(input, &result)
	}
}

func BenchmarkEncodeStream(b *testing.B) {
	c := New()
	data := map[string]string{"key": "value"}
	var buf bytes.Buffer
	for b.Loop() {
		buf.Reset()
		_ = c.EncodeStream(&buf, data)
	}
}
