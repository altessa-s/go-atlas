// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package requestid

import (
	"github.com/altessa-s/go-atlas/transport/internal/validation"

	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

// Generator extracts a request ID from a named header, validates it as a
// UUID v4, and optionally generates a new one when the value is absent or
// malformed. It is safe for concurrent use; all state is read-only after
// construction.
type Generator struct {
	opts       *options
	headerName string // Interned header name for performance
}

// NewGenerator creates a new Generator with the given options.
//
// Example:
//
//	gen := requestid.NewGenerator(
//		requestid.WithHeaderName("X-Request-ID"),
//	)
func NewGenerator(opt ...Option) *Generator {
	opts := newOptions(opt...)
	return &Generator{
		opts:       opts,
		headerName: corestrings.InternString(opts.headerName),
	}
}

// Extract extracts the request ID from headers or generates a new one.
// Returns the existing valid UUID v4 or generates a new one if missing/invalid.
//
// Example:
//
//	id := gen.Extract(headers)
func (g *Generator) Extract(headers HeaderGetter) string {
	var existingID string
	if headers != nil {
		existingID = headers.GetHeader(g.headerName)
	}
	return g.generate(existingID)
}

// generate processes an existing request ID and returns the appropriate ID.
func (g *Generator) generate(existingID string) string {
	if existingID != "" && validation.IsValidUUIDv4(existingID) {
		return existingID
	}

	if g.opts.generateIfMissing {
		return g.opts.uuidGenerator()
	}

	return ""
}

// HeaderName returns the configured header/metadata key name.
//
// Example:
//
//	name := gen.HeaderName()
func (g *Generator) HeaderName() string {
	return g.opts.headerName
}

// GenerateIfMissing returns whether new IDs are created for missing/invalid requests.
func (g *Generator) GenerateIfMissing() bool {
	return g.opts.generateIfMissing
}

// Options returns the generator options.
func (g *Generator) Options() *options {
	return g.opts
}
