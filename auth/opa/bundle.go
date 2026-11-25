// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package opa

import (
	"encoding/hex"
	"encoding/json"
	"iter"
	"maps"
	"slices"
	"time"

	"github.com/altessa-s/go-atlas-core/encoding/keys"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

// PolicyBundle represents a collection of OPA policy modules and optional data.
// It is the unit of policy delivery from a PolicySource.
type PolicyBundle struct {
	// Modules contains the policy modules keyed by their path.
	// Each value is the raw Rego content of the module.
	Modules map[string][]byte

	// Data contains optional JSON data to be loaded into OPA's data store.
	// Keys are paths (e.g., "users", "roles"), values are the parsed JSON data.
	Data map[string]any

	// Revision is a unique identifier for this version of the bundle.
	// Used for change detection. Typically a hash of the module contents.
	Revision string

	// FetchedAt is when this bundle was retrieved from the source.
	FetchedAt time.Time
}

// NewPolicyBundle creates a new PolicyBundle with the given modules.
// The revision is automatically calculated from the module contents.
func NewPolicyBundle(modules map[string][]byte) *PolicyBundle {
	// A module-only bundle has no JSON values to marshal.
	bundle, _ := NewPolicyBundleWithData(modules, nil)
	return bundle
}

// NewPolicyBundleWithData creates a new PolicyBundle with modules and data.
// The revision covers the modules and JSON data. Unencodable data returns an error.
func NewPolicyBundleWithData(modules map[string][]byte, data map[string]any) (*PolicyBundle, error) {
	bundle := &PolicyBundle{
		Modules:   modules,
		Data:      data,
		FetchedAt: time.Now(),
	}
	revision, err := bundle.calculateRevision()
	if err != nil {
		return nil, err
	}
	bundle.Revision = revision
	return bundle, nil
}

// calculateRevision computes a hash of all module contents for change detection.
func (b *PolicyBundle) calculateRevision() (string, error) {
	parts := make([][]byte, 0, 3*(len(b.Modules)+len(b.Data)))
	for _, key := range slices.Sorted(maps.Keys(b.Modules)) {
		parts = append(parts, []byte("module"), []byte(key), b.Modules[key])
	}
	for _, key := range slices.Sorted(maps.Keys(b.Data)) {
		raw, err := json.Marshal(b.Data[key])
		if err != nil {
			return "", coreerrs.Wrapf(err, "marshal data %q", key)
		}
		parts = append(parts, []byte("json"), []byte(key), raw)
	}
	return hex.EncodeToString(keys.Digest(parts...)), nil
}

// IsEmpty returns true if the bundle contains no modules.
func (b *PolicyBundle) IsEmpty() bool {
	return len(b.Modules) == 0
}

// ModuleCount returns the number of policy modules in the bundle.
func (b *PolicyBundle) ModuleCount() int {
	return len(b.Modules)
}

// ModulesIter returns an iterator over the policy modules.
// This provides lazy iteration without allocating a slice.
func (b *PolicyBundle) ModulesIter() iter.Seq2[string, []byte] {
	return maps.All(b.Modules)
}

// ModuleKeysIter returns an iterator over the module paths.
// This is more efficient than calling slices.Collect(maps.Keys(...)).
func (b *PolicyBundle) ModuleKeysIter() iter.Seq[string] {
	return maps.Keys(b.Modules)
}

// DataKeysIter returns an iterator over the data keys.
// Returns an empty iterator if no data is present.
func (b *PolicyBundle) DataKeysIter() iter.Seq[string] {
	if b.Data == nil {
		return func(yield func(string) bool) {}
	}
	return maps.Keys(b.Data)
}

// DataIter returns an iterator over the data entries.
// Returns an empty iterator if no data is present.
func (b *PolicyBundle) DataIter() iter.Seq2[string, any] {
	if b.Data == nil {
		return func(yield func(string, any) bool) {}
	}
	return maps.All(b.Data)
}
