// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package negcache

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

import "time"

// DefaultSharedRebuildCheckInterval is how often, at most, an unpopulated
// cache asks its filter whether a rebuild was committed elsewhere (see
// [WithSharedRebuildCheckInterval]).
const DefaultSharedRebuildCheckInterval = time.Second

// options carries the optional [Cache] tunables.
type options struct {
	// metrics records lookup outcomes. Nil disables metrics — every recording
	// becomes a no-op. Set via the generated WithMetrics.
	metrics *Metrics `optgen:"notnil"`

	// sharedRebuildCheckInterval throttles the shared-rebuild check of an
	// unpopulated cache whose filter implements
	// probfilter.RebuildCommitReporter: each check of a Redis filter costs a
	// round trip, so lookups check at most once per interval. Zero checks on
	// every unpopulated lookup; negative values are ignored. Set via the
	// generated WithSharedRebuildCheckInterval.
	sharedRebuildCheckInterval time.Duration `optgen:"default=DefaultSharedRebuildCheckInterval" optval:"positive=allow_zero"`
}
