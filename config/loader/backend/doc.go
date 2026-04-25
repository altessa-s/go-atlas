// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package backend defines interfaces for configuration file backends.
// Implement Backend to add support for new configuration formats.
//
// Example:
//
//	type JSONBackend struct{}
//	func (b *JSONBackend) Decode(r io.Reader, in any) error { return json.NewDecoder(r).Decode(in) }
//	func (b *JSONBackend) FileExtensions() []string { return []string{".json"} }
//	func (b *JSONBackend) StructTagName() string { return "json" }
package backend
