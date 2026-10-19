// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

import "github.com/altessa-s/go-atlas/core/time/clock"

// options configures a [Store].
type options struct {
	// clock seeds new namespaces and bounds bumps from below (default
	// clock.Real()); inject a fake clock in tests.
	clock clock.Clock `optgen:"default=clock.Real(),notnil"`
}
