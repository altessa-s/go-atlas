// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package checksum

import (
	coremaps "github.com/altessa-s/go-atlas/core/collections/maps"
	corehash "github.com/altessa-s/go-atlas/core/encoding/hash"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// Manifest owns an immutable snapshot of expected SHA-256 digests. A nil
// manifest disables verification; an empty manifest rejects every file.
type Manifest struct {
	entries *coremaps.ImmutableMap[string, string]
}

// New snapshots entries, preserving the distinction between nil and empty.
func New(entries map[string]string) *Manifest {
	if entries == nil {
		return nil
	}
	return &Manifest{entries: coremaps.NewImmutableMap(entries)}
}

// Verify checks one file, wrapping the source-specific sentinel on failure.
func (m *Manifest) Verify(path string, content []byte, mismatch, unexpected error) error {
	if m == nil {
		return nil
	}
	expected, ok := m.entries.Get(path)
	if !ok {
		return coreerrs.Wrapf(unexpected, "%s", path)
	}
	actual := corehash.SHA256HexBytes(content)
	if actual != expected {
		return coreerrs.Wrapf(mismatch, "%s (expected %s, got %s)", path, expected, actual)
	}
	return nil
}

// Covered checks that every expected file was loaded.
func (m *Manifest) Covered(loaded map[string]string, missing error) error {
	if m == nil {
		return nil
	}
	for path := range m.entries.All() {
		if _, ok := loaded[path]; !ok {
			return coreerrs.Wrapf(missing, "%s", path)
		}
	}
	return nil
}
