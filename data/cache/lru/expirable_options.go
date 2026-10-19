// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package lru

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate --type=expirableOptions --output=expirable_options_gen.go --option-type=ExpirableOption

import "time"

// expirableOptions configures [NewExpirableCache].
type expirableOptions struct {
	// clock reads the time entry expiry and cleanup are judged by (default time.Now); inject a fake clock in tests.
	clock func() time.Time `opt:"ExpirableClock" optgen:"default=time.Now"`
}
