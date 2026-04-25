// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package json

import (
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
	if c.indent {
		return c.encodeIndent(data)
	}
	return c.encodeCompact(data)
}

// Decode deserializes JSON bytes into out which must be a pointer.
func (c *Codec) Decode(data []byte, out any) error {
	return json.Unmarshal(data, out)
}

// ContentType returns the JSON MIME type.
func (c *Codec) ContentType() string {
	return MimeType
}

func (c *Codec) encodeCompact(data any) ([]byte, error) {
	if !c.escapeHTML {
		buf := coreio.GetBuffer()
		defer coreio.PutBuffer(buf)

		encoder := json.NewEncoder(buf)
		encoder.SetEscapeHTML(false)
		if err := encoder.Encode(data); err != nil {
			return nil, err
		}

		// Remove trailing newline added by encoder and copy result
		bytes := buf.Bytes()
		if len(bytes) > 0 && bytes[len(bytes)-1] == '\n' {
			bytes = bytes[:len(bytes)-1]
		}

		// Copy to avoid returning pooled buffer's backing array
		result := make([]byte, len(bytes))
		copy(result, bytes)
		return result, nil
	}

	return json.Marshal(data)
}

func (c *Codec) encodeIndent(data any) ([]byte, error) {
	if !c.escapeHTML {
		buf := coreio.GetBuffer()
		defer coreio.PutBuffer(buf)

		encoder := json.NewEncoder(buf)
		encoder.SetEscapeHTML(false)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(data); err != nil {
			return nil, err
		}

		// Remove trailing newline added by encoder and copy result
		bytes := buf.Bytes()
		if len(bytes) > 0 && bytes[len(bytes)-1] == '\n' {
			bytes = bytes[:len(bytes)-1]
		}

		// Copy to avoid returning pooled buffer's backing array
		result := make([]byte, len(bytes))
		copy(result, bytes)
		return result, nil
	}

	return json.MarshalIndent(data, "", "  ")
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
