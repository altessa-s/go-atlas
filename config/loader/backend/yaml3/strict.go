// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package yaml3

import (
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"

	"gopkg.in/yaml.v3"

	"github.com/altessa-s/go-atlas/config/loader/backend"
)

// unknownFieldMsg matches the message yaml.v3 records for a mapping key that
// binds to no field of a struct when KnownFields is enabled.
var unknownFieldMsg = regexp.MustCompile(`^line \d+: field .* not found in type `)

// DecodeStrict decodes the YAML data from the reader like Decode, but rejects
// mapping keys that bind to no field of a destination struct. yaml.v3 keeps
// decoding past such keys, so the returned error names all of them; it wraps
// [backend.ErrUnknownField] and the underlying *yaml.TypeError.
//
// Map entry keys are free, though struct values in a map are checked; an
// ",inline" map absorbs every key its struct lacks, and content under
// interfaces and types implementing yaml.Unmarshaler is free-form. A custom unmarshaler that calls node.Decode decodes
// its value without the check.
func (b *Backend) DecodeStrict(reader io.Reader, in any) error {
	decoder := yaml.NewDecoder(reader)
	decoder.KnownFields(true)
	err := decoder.Decode(in)
	if te, ok := errors.AsType[*yaml.TypeError](err); ok && hasUnknownField(te) {
		return fmt.Errorf("%w: %w", backend.ErrUnknownField, err)
	}
	return err
}

// hasUnknownField reports whether te records an unknown struct field.
func hasUnknownField(te *yaml.TypeError) bool {
	return slices.ContainsFunc(te.Errors, unknownFieldMsg.MatchString)
}

// Ensure Backend implements backend.StrictDecoder.
var _ backend.StrictDecoder = (*Backend)(nil)
