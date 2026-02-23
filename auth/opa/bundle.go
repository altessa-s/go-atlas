// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package opa

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"iter"
	"maps"
	"slices"
	"time"
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
	bundle := &PolicyBundle{
		Modules:   modules,
		FetchedAt: time.Now(),
	}
	bundle.Revision = bundle.calculateRevision()
	return bundle
}

// NewPolicyBundleWithData creates a new PolicyBundle with modules and data.
// The revision is automatically calculated from the module contents.
func NewPolicyBundleWithData(modules map[string][]byte, data map[string]any) *PolicyBundle {
	bundle := &PolicyBundle{
		Modules:   modules,
		Data:      data,
		FetchedAt: time.Now(),
	}
	bundle.Revision = bundle.calculateRevision()
	return bundle
}

// calculateRevision computes a hash of all module contents for change detection.
func (b *PolicyBundle) calculateRevision() string {
	h := sha256.New()

	// Sort keys for deterministic ordering
	keys := slices.Collect(maps.Keys(b.Modules))
	slices.Sort(keys)

	for _, key := range keys {
		h.Write([]byte(key))
		h.Write(b.Modules[key])
	}

	if len(b.Data) > 0 {
		dataKeys := slices.Collect(maps.Keys(b.Data))
		slices.Sort(dataKeys)
		for _, key := range dataKeys {
			h.Write([]byte(key))
			if raw, err := json.Marshal(b.Data[key]); err == nil {
				h.Write(raw)
			} else {
				h.Write([]byte("marshal_error:" + err.Error()))
			}
		}
	}

	return hex.EncodeToString(h.Sum(nil))[:16]
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
