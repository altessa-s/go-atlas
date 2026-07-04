// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mirror

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

// options carries the optional [Cache] tunables.
type options struct {
	// metrics records refresh outcomes and snapshot size. Nil disables metrics —
	// every recording becomes a no-op. Set via the generated WithMetrics.
	metrics *Metrics `optgen:"notnil"`
}
