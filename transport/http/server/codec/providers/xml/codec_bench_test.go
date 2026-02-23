// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package xml

import (
	"encoding/xml"
	"testing"
)

type benchItem struct {
	XMLName xml.Name `xml:"Item"`
	Name    string   `xml:"Name"`
	Value   int      `xml:"Value"`
}

func BenchmarkEncode_Compact(b *testing.B) {
	c := New()
	item := benchItem{Name: "test", Value: 42}
	var result []byte
	for b.Loop() {
		result, _ = c.Encode(item)
	}
	_ = result
}

func BenchmarkEncode_Indent(b *testing.B) {
	c := New(WithIndent(true))
	item := benchItem{Name: "test", Value: 42}
	var result []byte
	for b.Loop() {
		result, _ = c.Encode(item)
	}
	_ = result
}

func BenchmarkEncode_WithHeader(b *testing.B) {
	c := New(WithHeader(true))
	item := benchItem{Name: "test", Value: 42}
	var result []byte
	for b.Loop() {
		result, _ = c.Encode(item)
	}
	_ = result
}

func BenchmarkDecode(b *testing.B) {
	c := New()
	input := []byte(`<Item><Name>test</Name><Value>42</Value></Item>`)
	for b.Loop() {
		var result benchItem
		_ = c.Decode(input, &result)
	}
}
