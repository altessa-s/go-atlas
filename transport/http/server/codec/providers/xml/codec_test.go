// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package xml

import (
	"bytes"
	"encoding/xml"
	"testing"
)

type testItem struct {
	XMLName xml.Name `xml:"Item"`
	Name    string   `xml:"Name"`
	Value   int      `xml:"Value"`
}

func TestNew(t *testing.T) {
	c := New()
	if c == nil {
		t.Fatal("New() returned nil")
	}
	if c.indent {
		t.Fatal("indent should default to false")
	}
	if c.includeHeader {
		t.Fatal("includeHeader should default to false")
	}
}

func TestNew_WithOptions(t *testing.T) {
	c := New(WithIndent(true), WithHeader(true))
	if !c.indent {
		t.Fatal("indent should be true")
	}
	if !c.includeHeader {
		t.Fatal("includeHeader should be true")
	}
}

func TestContentType(t *testing.T) {
	c := New()
	if c.ContentType() != MimeType {
		t.Fatalf("ContentType() = %q, want %q", c.ContentType(), MimeType)
	}
}

func TestEncode_Compact(t *testing.T) {
	c := New()
	item := testItem{Name: "test", Value: 42}
	b, err := c.Encode(item)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	if !bytes.Contains(b, []byte("<Name>test</Name>")) {
		t.Fatalf("output = %s", b)
	}
}

func TestEncode_Indent(t *testing.T) {
	c := New(WithIndent(true))
	item := testItem{Name: "test", Value: 42}
	b, err := c.Encode(item)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	if !bytes.Contains(b, []byte("\n")) {
		t.Fatal("indented output should contain newlines")
	}
}

func TestEncode_WithHeader(t *testing.T) {
	c := New(WithHeader(true))
	item := testItem{Name: "test", Value: 1}
	b, err := c.Encode(item)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	if !bytes.HasPrefix(b, []byte(XMLHeader)) {
		t.Fatalf("output should start with XML header, got %s", b[:50])
	}
}

func TestEncode_WithoutHeader(t *testing.T) {
	c := New()
	item := testItem{Name: "test", Value: 1}
	b, err := c.Encode(item)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	if bytes.Contains(b, []byte("<?xml")) {
		t.Fatal("output should not contain XML header")
	}
}

func TestDecode(t *testing.T) {
	c := New()
	input := []byte(`<Item><Name>test</Name><Value>42</Value></Item>`)
	var result testItem
	if err := c.Decode(input, &result); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if result.Name != "test" {
		t.Fatalf("Name = %q", result.Name)
	}
	if result.Value != 42 {
		t.Fatalf("Value = %d", result.Value)
	}
}

func TestDecode_Invalid(t *testing.T) {
	c := New()
	var result testItem
	if err := c.Decode([]byte("not xml <>>"), &result); err == nil {
		t.Fatal("expected error for invalid XML")
	}
}

func TestEncodeDecode_Roundtrip(t *testing.T) {
	c := New()
	original := testItem{Name: "roundtrip", Value: 99}
	encoded, err := c.Encode(original)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	var decoded testItem
	if err := c.Decode(encoded, &decoded); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if decoded.Name != original.Name || decoded.Value != original.Value {
		t.Fatalf("roundtrip mismatch: got %+v, want %+v", decoded, original)
	}
}
