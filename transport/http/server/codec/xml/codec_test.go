// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package xml

import (
	"bytes"
	"encoding/xml"
	"testing"

	"github.com/stretchr/testify/require"
)

type testItem struct {
	XMLName xml.Name `xml:"Item"`
	Name    string   `xml:"Name"`
	Value   int      `xml:"Value"`
}

func TestNew(t *testing.T) {
	c := New()
	require.NotNil(t, c)
	require.False(t, c.indent, "indent should default to false")
	require.False(t, c.includeHeader, "includeHeader should default to false")
}

func TestNew_WithOptions(t *testing.T) {
	c := New(WithIndent(true), WithHeader(true))
	require.True(t, c.indent, "indent should be true")
	require.True(t, c.includeHeader, "includeHeader should be true")
}

func TestContentType(t *testing.T) {
	c := New()
	require.Equal(t, MimeType, c.ContentType())
}

func TestEncode_Compact(t *testing.T) {
	c := New()
	item := testItem{Name: "test", Value: 42}
	b, err := c.Encode(item)
	require.NoError(t, err)
	require.True(t, bytes.Contains(b, []byte("<Name>test</Name>")), "output = %s", b)
}

func TestEncode_Indent(t *testing.T) {
	c := New(WithIndent(true))
	item := testItem{Name: "test", Value: 42}
	b, err := c.Encode(item)
	require.NoError(t, err)
	require.True(t, bytes.Contains(b, []byte("\n")), "indented output should contain newlines")
}

func TestEncode_WithHeader(t *testing.T) {
	c := New(WithHeader(true))
	item := testItem{Name: "test", Value: 1}
	b, err := c.Encode(item)
	require.NoError(t, err)
	require.True(t, bytes.HasPrefix(b, []byte(XMLHeader)), "output should start with XML header, got %s", b[:50])
}

func TestEncode_WithoutHeader(t *testing.T) {
	c := New()
	item := testItem{Name: "test", Value: 1}
	b, err := c.Encode(item)
	require.NoError(t, err)
	require.False(t, bytes.Contains(b, []byte("<?xml")), "output should not contain XML header")
}

func TestDecode(t *testing.T) {
	c := New()
	input := []byte(`<Item><Name>test</Name><Value>42</Value></Item>`)
	var result testItem
	err := c.Decode(input, &result)
	require.NoError(t, err)
	require.Equal(t, "test", result.Name)
	require.Equal(t, 42, result.Value)
}

func TestDecode_Invalid(t *testing.T) {
	c := New()
	var result testItem
	err := c.Decode([]byte("not xml <>>"), &result)
	require.Error(t, err)
}

func TestEncodeDecode_Roundtrip(t *testing.T) {
	c := New()
	original := testItem{Name: "roundtrip", Value: 99}
	encoded, err := c.Encode(original)
	require.NoError(t, err)
	var decoded testItem
	err = c.Decode(encoded, &decoded)
	require.NoError(t, err)
	require.Equal(t, original.Name, decoded.Name)
	require.Equal(t, original.Value, decoded.Value)
}
