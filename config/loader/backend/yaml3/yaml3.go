// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package yaml3

import (
	"io"

	"gopkg.in/yaml.v3"

	"github.com/altessa-s/go-atlas/config/loader/backend"
)

// Backend is a YAML file backend for the configuration parser.
// It uses gopkg.in/yaml.v3 for parsing.
//
// Example:
//
//	p := parser.New(&yaml3.Backend{}, parser.WithPath("config.yaml"))
type Backend struct{}

// StructTagName returns the struct tag name for YAML.
func (b *Backend) StructTagName() string {
	return "yaml"
}

// FileExtensions returns the YAML file extensions.
func (b *Backend) FileExtensions() []string {
	return []string{"yaml", "yml"}
}

// Decode decodes the YAML data from the reader.
func (b *Backend) Decode(reader io.Reader, in any) error {
	decoder := yaml.NewDecoder(reader)
	return decoder.Decode(in)
}

// Ensure Backend implements backend.Backend.
var _ backend.Backend = (*Backend)(nil)
