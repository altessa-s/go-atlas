// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package stats

import "time"

// FilterStats contains statistics about a probabilistic filter.
type FilterStats struct {
	// Capacity is the maximum number of items the filter can hold.
	Capacity int64 `json:"capacity"`

	// ItemCount is the approximate number of items in the filter.
	ItemCount int64 `json:"itemCount"`

	// FillRatio is the ratio of used capacity (0.0 to 1.0).
	FillRatio float64 `json:"fillRatio"`

	// FalsePositiveRate is the estimated false positive rate.
	// For Bloom filters, this is based on the current fill ratio.
	// For Cuckoo filters, this is based on fingerprint size.
	FalsePositiveRate float64 `json:"falsePositiveRate"`

	// MemoryUsageBytes is the memory used by the filter in bytes.
	// May be zero for remote storage backends.
	MemoryUsageBytes int64 `json:"memoryUsageBytes,omitempty"`

	// LastRebuild is the time of the last filter rebuild.
	// Only applicable for RebuildableFilter implementations.
	LastRebuild time.Time `json:"lastRebuild,omitzero"`

	// StorageType indicates the storage backend (e.g., "memory", "redis").
	StorageType string `json:"storageType"`
}
