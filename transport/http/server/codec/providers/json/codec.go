// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package json

import (
	"bytes"
	"encoding/json"
	"io"

	coreio "github.com/altessa-s/go-atlas/core/io"
)

// Compile-time interface checks.
var (
	_ interface {
		Encode(data any) ([]byte, error)
		Decode(data []byte, out any) error
		ContentType() string
	} = (*Codec)(nil)
	_ interface{ EncodeStream(io.Writer, any) error } = (*Codec)(nil)
)

// MimeType is the primary MIME type for JSON.
const MimeType = "application/json"

// AlternateMimeType is an alternate MIME type for JSON.
const AlternateMimeType = "text/json"

// Codec is a JSON codec using encoding/json.
// It is safe for concurrent use.
type Codec struct {
	indent     bool
	escapeHTML bool
}

// Option is a functional option for configuring the Codec.
type Option func(*Codec)

// WithIndent enables pretty-printing with 2-space indentation.
func WithIndent(indent bool) Option {
	return func(c *Codec) {
		c.indent = indent
	}
}

// WithEscapeHTML controls HTML character escaping.
func WithEscapeHTML(escape bool) Option {
	return func(c *Codec) {
		c.escapeHTML = escape
	}
}

// New creates a new JSON codec with default settings.
//
// Example:
//
//	codec := json.New(json.WithIndent(true))
func New(opts ...Option) *Codec {
	c := &Codec{
		indent:     false,
		escapeHTML: true,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// Encode serializes data into JSON bytes.
func (c *Codec) Encode(data any) ([]byte, error) {
	if !c.escapeHTML {
		return c.encodeUnescaped(data)
	}
	if c.indent {
		return json.MarshalIndent(data, "", "  ")
	}
	return json.Marshal(data)
}

// Decode deserializes JSON bytes into out which must be a pointer.
func (c *Codec) Decode(data []byte, out any) error {
	return json.Unmarshal(data, out)
}

// ContentType returns the JSON MIME type.
func (c *Codec) ContentType() string {
	return MimeType
}

// encodeUnescaped encodes through a pooled [json.Encoder], the only stdlib
// path that can disable HTML escaping.
func (c *Codec) encodeUnescaped(data any) ([]byte, error) {
	buf := coreio.GetBuffer()
	defer coreio.PutBuffer(buf)

	encoder := json.NewEncoder(buf)
	encoder.SetEscapeHTML(false)
	if c.indent {
		encoder.SetIndent("", "  ")
	}
	if err := encoder.Encode(data); err != nil {
		return nil, err
	}

	// Drop the encoder's trailing newline and copy out of the pooled buffer.
	return bytes.Clone(bytes.TrimSuffix(buf.Bytes(), []byte{'\n'})), nil
}

// EncodeStream writes JSON directly to w without buffering the entire response.
// This implements the codec.StreamingEncoder interface for memory-efficient
// encoding of large responses.
func (c *Codec) EncodeStream(w io.Writer, data any) error {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(c.escapeHTML)
	if c.indent {
		encoder.SetIndent("", "  ")
	}
	return encoder.Encode(data)
}
