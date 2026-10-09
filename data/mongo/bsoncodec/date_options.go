// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package bsoncodec

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate --type=dateOptions --option-type=DateOption --output=date_options_gen.go --option-prefix=Date

// dateOptions configures a date codec registered with [RegisterDateType].
type dateOptions struct {
	// zeroAsEmptyString stores the zero date as "" instead of null, for
	// collections that already hold empty strings for "no date".
	zeroAsEmptyString bool
}
