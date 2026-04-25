// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package xml

import (
	"encoding/xml"

	coreio "github.com/altessa-s/go-atlas/core/io"
)

// Compile-time interface check.
var _ interface {
	Encode(data any) ([]byte, error)
	Decode(data []byte, out any) error
	ContentType() string
} = (*Codec)(nil)

// MimeType is the primary MIME type for XML.
const MimeType = "application/xml"

// AlternateMimeType is an alternate MIME type for XML.
const AlternateMimeType = "text/xml"

// XMLHeader is the standard XML declaration.
const XMLHeader = `<?xml version="1.0" encoding="UTF-8"?>`

// Codec is an XML codec using encoding/xml.
// It is safe for concurrent use.
type Codec struct {
	indent        bool
	includeHeader bool
}

// Option is a functional option for configuring the Codec.
type Option func(*Codec)

// WithIndent enables pretty-printing with 2-space indentation.
func WithIndent(indent bool) Option {
	return func(c *Codec) {
		c.indent = indent
	}
}

// WithHeader controls inclusion of the XML declaration header.
func WithHeader(include bool) Option {
	return func(c *Codec) {
		c.includeHeader = include
	}
}

// New creates a new XML codec with default settings.
//
// Example:
//
//	codec := xml.New(xml.WithIndent(true))
func New(opts ...Option) *Codec {
	c := &Codec{
		indent:        false,
		includeHeader: false,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// Encode serializes data into XML bytes.
func (c *Codec) Encode(data any) ([]byte, error) {
	var result []byte
	var err error

	if c.indent {
		result, err = xml.MarshalIndent(data, "", "  ")
	} else {
		result, err = xml.Marshal(data)
	}

	if err != nil {
		return nil, err
	}

	if !c.includeHeader {
		return result, nil
	}

	// Add XML header using buffer pool
	buf := coreio.GetBuffer()
	defer coreio.PutBuffer(buf)

	buf.WriteString(XMLHeader)
	buf.WriteByte('\n')
	buf.Write(result)

	// Copy to avoid returning pooled buffer's backing array
	output := make([]byte, buf.Len())
	copy(output, buf.Bytes())
	return output, nil
}

// Decode deserializes XML bytes into out which must be a pointer.
func (c *Codec) Decode(data []byte, out any) error {
	return xml.Unmarshal(data, out)
}

// ContentType returns the XML MIME type.
func (c *Codec) ContentType() string {
	return MimeType
}
