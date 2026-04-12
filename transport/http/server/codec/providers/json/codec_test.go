// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package json

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	c := New()
	require.NotNil(t, c)
	require.False(t, c.indent, "indent should default to false")
	require.True(t, c.escapeHTML, "escapeHTML should default to true")
}

func TestNew_WithOptions(t *testing.T) {
	c := New(WithIndent(true), WithEscapeHTML(false))
	require.True(t, c.indent, "indent should be true")
	require.False(t, c.escapeHTML, "escapeHTML should be false")
}

func TestContentType(t *testing.T) {
	c := New()
	require.Equal(t, MimeType, c.ContentType())
}

func TestEncode_Compact(t *testing.T) {
	c := New()
	data := map[string]string{"key": "value"}
	b, err := c.Encode(data)
	require.NoError(t, err)
	var result map[string]string
	require.NoError(t, json.Unmarshal(b, &result))
	require.Equal(t, "value", result["key"])
}

func TestEncode_Indent(t *testing.T) {
	c := New(WithIndent(true))
	data := map[string]string{"key": "value"}
	b, err := c.Encode(data)
	require.NoError(t, err)
	require.True(t, bytes.Contains(b, []byte("\n")), "indented output should contain newlines")
}

func TestEncode_NoEscapeHTML(t *testing.T) {
	c := New(WithEscapeHTML(false))
	data := map[string]string{"html": "<b>bold</b>"}
	b, err := c.Encode(data)
	require.NoError(t, err)
	require.True(t, bytes.Contains(b, []byte("<b>")), "HTML should not be escaped")
}

func TestEncode_EscapeHTML(t *testing.T) {
	c := New(WithEscapeHTML(true))
	data := map[string]string{"html": "<b>bold</b>"}
	b, err := c.Encode(data)
	require.NoError(t, err)
	require.False(t, bytes.Contains(b, []byte("<b>")), "HTML should be escaped")
}

func TestEncode_IndentNoEscapeHTML(t *testing.T) {
	c := New(WithIndent(true), WithEscapeHTML(false))
	data := map[string]string{"html": "<b>test</b>"}
	b, err := c.Encode(data)
	require.NoError(t, err)
	require.True(t, bytes.Contains(b, []byte("<b>")), "HTML should not be escaped")
	require.True(t, bytes.Contains(b, []byte("\n")), "should be indented")
}

func TestDecode(t *testing.T) {
	c := New()
	input := []byte(`{"name":"test","count":42}`)
	var result struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}
	err := c.Decode(input, &result)
	require.NoError(t, err)
	require.Equal(t, "test", result.Name)
	require.Equal(t, 42, result.Count)
}

func TestDecode_Invalid(t *testing.T) {
	c := New()
	var result map[string]any
	err := c.Decode([]byte("not json"), &result)
	require.Error(t, err)
}

func TestEncodeStream(t *testing.T) {
	c := New()
	var buf bytes.Buffer
	data := map[string]string{"key": "value"}
	err := c.EncodeStream(&buf, data)
	require.NoError(t, err)
	var result map[string]string
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	require.Equal(t, "value", result["key"])
}

func TestEncodeStream_Indent(t *testing.T) {
	c := New(WithIndent(true))
	var buf bytes.Buffer
	err := c.EncodeStream(&buf, map[string]string{"k": "v"})
	require.NoError(t, err)
	require.True(t, bytes.Contains(buf.Bytes(), []byte("\n")), "stream output should be indented")
}

func TestEncode_Error(t *testing.T) {
	c := New()
	// channels cannot be marshaled
	_, err := c.Encode(make(chan int))
	require.Error(t, err)
}

func TestEncode_IndentError(t *testing.T) {
	c := New(WithIndent(true))
	_, err := c.Encode(make(chan int))
	require.Error(t, err)
}
