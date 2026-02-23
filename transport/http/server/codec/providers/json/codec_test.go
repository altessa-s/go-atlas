// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package json

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestNew(t *testing.T) {
	c := New()
	if c == nil {
		t.Fatal("New() returned nil")
	}
	if c.indent {
		t.Fatal("indent should default to false")
	}
	if !c.escapeHTML {
		t.Fatal("escapeHTML should default to true")
	}
}

func TestNew_WithOptions(t *testing.T) {
	c := New(WithIndent(true), WithEscapeHTML(false))
	if !c.indent {
		t.Fatal("indent should be true")
	}
	if c.escapeHTML {
		t.Fatal("escapeHTML should be false")
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
	data := map[string]string{"key": "value"}
	b, err := c.Encode(data)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	var result map[string]string
	if err := json.Unmarshal(b, &result); err != nil {
		t.Fatalf("Unmarshal error = %v", err)
	}
	if result["key"] != "value" {
		t.Fatalf("key = %q", result["key"])
	}
}

func TestEncode_Indent(t *testing.T) {
	c := New(WithIndent(true))
	data := map[string]string{"key": "value"}
	b, err := c.Encode(data)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	if !bytes.Contains(b, []byte("\n")) {
		t.Fatal("indented output should contain newlines")
	}
}

func TestEncode_NoEscapeHTML(t *testing.T) {
	c := New(WithEscapeHTML(false))
	data := map[string]string{"html": "<b>bold</b>"}
	b, err := c.Encode(data)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	if !bytes.Contains(b, []byte("<b>")) {
		t.Fatal("HTML should not be escaped")
	}
}

func TestEncode_EscapeHTML(t *testing.T) {
	c := New(WithEscapeHTML(true))
	data := map[string]string{"html": "<b>bold</b>"}
	b, err := c.Encode(data)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	if bytes.Contains(b, []byte("<b>")) {
		t.Fatal("HTML should be escaped")
	}
}

func TestEncode_IndentNoEscapeHTML(t *testing.T) {
	c := New(WithIndent(true), WithEscapeHTML(false))
	data := map[string]string{"html": "<b>test</b>"}
	b, err := c.Encode(data)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	if !bytes.Contains(b, []byte("<b>")) {
		t.Fatal("HTML should not be escaped")
	}
	if !bytes.Contains(b, []byte("\n")) {
		t.Fatal("should be indented")
	}
}

func TestDecode(t *testing.T) {
	c := New()
	input := []byte(`{"name":"test","count":42}`)
	var result struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}
	if err := c.Decode(input, &result); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if result.Name != "test" {
		t.Fatalf("Name = %q", result.Name)
	}
	if result.Count != 42 {
		t.Fatalf("Count = %d", result.Count)
	}
}

func TestDecode_Invalid(t *testing.T) {
	c := New()
	var result map[string]any
	if err := c.Decode([]byte("not json"), &result); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestEncodeStream(t *testing.T) {
	c := New()
	var buf bytes.Buffer
	data := map[string]string{"key": "value"}
	if err := c.EncodeStream(&buf, data); err != nil {
		t.Fatalf("EncodeStream() error = %v", err)
	}
	var result map[string]string
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("Unmarshal error = %v", err)
	}
	if result["key"] != "value" {
		t.Fatalf("key = %q", result["key"])
	}
}

func TestEncodeStream_Indent(t *testing.T) {
	c := New(WithIndent(true))
	var buf bytes.Buffer
	if err := c.EncodeStream(&buf, map[string]string{"k": "v"}); err != nil {
		t.Fatalf("EncodeStream() error = %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("\n")) {
		t.Fatal("stream output should be indented")
	}
}

func TestConstants(t *testing.T) {
	if MimeType != "application/json" {
		t.Fatalf("MimeType = %q", MimeType)
	}
	if AlternateMimeType != "text/json" {
		t.Fatalf("AlternateMimeType = %q", AlternateMimeType)
	}
}

func TestEncode_Error(t *testing.T) {
	c := New()
	// channels cannot be marshaled
	_, err := c.Encode(make(chan int))
	if err == nil {
		t.Fatal("expected error for unmarshalable type")
	}
}

func TestEncode_IndentError(t *testing.T) {
	c := New(WithIndent(true))
	_, err := c.Encode(make(chan int))
	if err == nil {
		t.Fatal("expected error for unmarshalable type")
	}
}
